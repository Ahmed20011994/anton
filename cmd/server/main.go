package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/Ahmed20011994/anton/internal/config"
	"github.com/Ahmed20011994/anton/internal/db"
	"github.com/Ahmed20011994/anton/internal/handler"
	"github.com/Ahmed20011994/anton/internal/middleware"
	"github.com/Ahmed20011994/anton/internal/repository"
	"github.com/Ahmed20011994/anton/internal/service"
	"github.com/Ahmed20011994/anton/internal/worker"
)

const staleHeartbeatThreshold = 90 * time.Second

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer bootCancel()

	pool, err := db.NewPool(bootCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db pool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(bootCtx, pool); err != nil {
		logger.Error("migrate", "err", err)
		os.Exit(1)
	}

	tenantRepo := repository.NewTenantRepo(pool)
	integRepo := repository.NewIntegrationRepo(pool)
	workItemRepo := repository.NewWorkItemRepo(pool)
	jobRepo := repository.NewSyncJobRepo(pool)

	integSvc := service.NewIntegrationService(integRepo, cfg.EncryptionKey)
	syncSvc := service.NewSyncService(integSvc, integRepo, workItemRepo, jobRepo, logger)

	// Recover any jobs that were running when the previous instance died.
	reclaimed, err := jobRepo.ReclaimStale(bootCtx, staleHeartbeatThreshold)
	if err != nil {
		logger.Error("reclaim stale jobs", "err", err)
	} else if reclaimed > 0 {
		logger.Info("reclaimed stale jobs", "count", reclaimed)
	}

	healthHandler := handler.NewHealthHandler()
	integHandler := handler.NewIntegrationHandler(integSvc)
	syncHandler := handler.NewSyncHandler(syncSvc)
	workItemHandler := handler.NewWorkItemHandler(workItemRepo)
	syncJobsHandler := handler.NewSyncJobsHandler(jobRepo)

	requireTenant := middleware.RequireTenant(tenantRepo)
	protected := func(h http.Handler) http.Handler {
		return requireTenant(middleware.RequireAPIKey(h))
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", healthHandler)
	mux.Handle("POST /v1/tenants/{slug}/integrations/{source}",
		protected(http.HandlerFunc(integHandler.Put)))
	mux.Handle("POST /v1/tenants/{slug}/sync/{source}",
		protected(http.HandlerFunc(syncHandler.Enqueue)))
	mux.Handle("GET /v1/tenants/{slug}/sync-jobs",
		protected(http.HandlerFunc(syncJobsHandler.List)))
	mux.Handle("GET /v1/tenants/{slug}/sync-jobs/{job_id}",
		protected(http.HandlerFunc(syncJobsHandler.Get)))
	mux.Handle("GET /v1/tenants/{slug}/work-items",
		protected(http.HandlerFunc(workItemHandler.List)))

	rootHandler := middleware.Logging(logger)(mux)

	// Start worker pool in its own context so we can cancel + drain on shutdown.
	host, _ := os.Hostname()
	if host == "" {
		host = "anton"
	}
	workerID := fmt.Sprintf("%s.%d", host, os.Getpid())
	workerCtx, workerCancel := context.WithCancel(context.Background())
	workerPool := worker.NewPool(jobRepo, syncSvc, cfg.WorkerCount, workerID, logger)
	workerPool.Start(workerCtx)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           rootHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		logger.Info("server starting", "port", cfg.Port, "env", cfg.Env, "workers", cfg.WorkerCount)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	// Stop accepting new HTTP requests, then drain workers, then close the pool.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "err", err)
	}

	workerCancel()
	workerPool.Wait()
	logger.Info("server stopped")
}
