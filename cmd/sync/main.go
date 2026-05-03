package main

import (
	"context"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/Ahmed20011994/anton/internal/config"
	"github.com/Ahmed20011994/anton/internal/db"
	"github.com/Ahmed20011994/anton/internal/repository"
	"github.com/Ahmed20011994/anton/internal/service"
	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

// cmd/sync enqueues sync jobs for every tenant × enabled integration.
// Workers running inside cmd/server pick the jobs up and execute them.
// Cron triggers this binary; the actual sync work happens in the server.

func main() {
	tenantSlug := flag.String("tenant", "", "limit to a single tenant slug (default: all)")
	sourceType := flag.String("source", "", "limit to a single source type (default: all enabled)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db pool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		logger.Error("migrate", "err", err)
		os.Exit(1)
	}

	tenantRepo := repository.NewTenantRepo(pool)
	integRepo := repository.NewIntegrationRepo(pool)
	workItemRepo := repository.NewWorkItemRepo(pool)
	jobRepo := repository.NewSyncJobRepo(pool)
	integSvc := service.NewIntegrationService(integRepo, cfg.EncryptionKey)
	syncSvc := service.NewSyncService(integSvc, integRepo, workItemRepo, jobRepo, logger)

	tenants, err := tenantRepo.ListAll(ctx)
	if err != nil {
		logger.Error("list tenants", "err", err)
		os.Exit(1)
	}

	type result struct {
		Tenant string                `json:"tenant"`
		Jobs   []repository.SyncJob  `json:"jobs"`
	}
	out := make([]result, 0)

	for _, t := range tenants {
		if *tenantSlug != "" && t.Slug != *tenantSlug {
			continue
		}
		scope := tenantctx.Scope{TenantID: t.ID, Slug: t.Slug}

		integs, err := integRepo.ListEnabledForTenant(ctx, t.ID)
		if err != nil {
			logger.Error("list integrations", "tenant", t.Slug, "err", err)
			continue
		}

		var enqueued []repository.SyncJob
		for _, i := range integs {
			if *sourceType != "" && i.SourceType != *sourceType {
				continue
			}
			job, err := syncSvc.Enqueue(ctx, scope, i.SourceType)
			if err != nil {
				logger.Error("enqueue", "tenant", t.Slug, "source", i.SourceType, "err", err)
				continue
			}
			logger.Info("enqueued",
				"tenant", t.Slug, "source", i.SourceType, "job_id", job.ID)
			enqueued = append(enqueued, job)
		}
		if len(enqueued) > 0 {
			out = append(out, result{Tenant: t.Slug, Jobs: enqueued})
		}
	}

	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		logger.Error("encode summary", "err", err)
	}
}
