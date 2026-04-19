package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/Ahmed20011994/anton/internal/config"
	"github.com/Ahmed20011994/anton/internal/handler"
	"github.com/Ahmed20011994/anton/internal/middleware"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg := config.Load()

	mux := http.NewServeMux()

	healthHandler := handler.NewHealthHandler()
	mux.Handle("GET /healthz", healthHandler)

	loggedMux := middleware.Logging(logger)(mux)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: loggedMux,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		logger.Info("server starting", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "err", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
