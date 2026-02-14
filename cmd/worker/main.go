package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dunamismax/pixelflow/internal/config"
	"github.com/dunamismax/pixelflow/internal/pipeline"
	"github.com/dunamismax/pixelflow/internal/storage"
	"github.com/dunamismax/pixelflow/internal/store"
	"github.com/dunamismax/pixelflow/internal/telemetry"
	"github.com/dunamismax/pixelflow/internal/webhook"
	"github.com/dunamismax/pixelflow/internal/worker"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "[worker] ", log.LstdFlags|log.Lmsgprefix)

	traceShutdown, err := telemetry.SetupTracing(context.Background(), telemetry.TraceConfig{
		ServiceName:  "pixelflow-worker",
		Exporter:     cfg.Telemetry.TracesExporter,
		OTLPEndpoint: cfg.Telemetry.OTLPTraceEndpoint,
		OTLPInsecure: cfg.Telemetry.OTLPInsecure,
	}, logger)
	if err != nil {
		logger.Fatalf("tracing init failed: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := traceShutdown(shutdownCtx); err != nil {
			logger.Printf("tracing shutdown error: %v", err)
		}
	}()

	logger.Printf(
		"starting worker concurrency=%d max_active_jobs=%d queue=%s redis=%s",
		cfg.Worker.Concurrency,
		cfg.Worker.MaxActiveJobs,
		cfg.Queue.Name,
		cfg.Queue.RedisAddr,
	)

	if err := pipeline.Startup(); err != nil {
		logger.Fatalf("pipeline runtime startup failed: %v", err)
	}
	defer pipeline.Shutdown()

	logger.Printf("local output dir=%s", cfg.Worker.LocalOutputDir)

	storageClient, err := storage.NewClient(storage.Config{
		Endpoint: cfg.Storage.Endpoint,
		Access:   cfg.Storage.AccessKey,
		Secret:   cfg.Storage.SecretKey,
		Bucket:   cfg.Storage.Bucket,
		UseSSL:   cfg.Storage.UseSSL,
	})
	if err != nil {
		logger.Fatalf("storage init failed: %v", err)
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startupCancel()

	if err := storageClient.EnsureBucket(startupCtx); err != nil {
		logger.Fatalf("storage bucket check failed: %v", err)
	}

	webhookClient := webhook.NewClient(webhook.Config{
		SigningSecret:  cfg.Webhook.SigningSecret,
		Timeout:        cfg.Webhook.Timeout,
		MaxAttempts:    cfg.Webhook.MaxAttempts,
		InitialBackoff: cfg.Webhook.InitialBackoff,
		MaxBackoff:     cfg.Webhook.MaxBackoff,
	})

	jobStore, err := store.NewPostgresJobStore(startupCtx, cfg.Database.DSN)
	if err != nil {
		logger.Fatalf("job store init failed: %v", err)
	}
	defer func() {
		if err := jobStore.Close(); err != nil {
			logger.Printf("job store close error: %v", err)
		}
	}()

	srv, err := worker.NewServer(logger, cfg.Queue, cfg.Worker, storageClient, webhookClient, jobStore, jobStore)
	if err != nil {
		logger.Fatalf("worker init failed: %v", err)
	}

	var metricsServer *http.Server
	if strings.TrimSpace(cfg.Worker.MetricsAddr) != "" {
		metricsServer = &http.Server{
			Addr:         cfg.Worker.MetricsAddr,
			Handler:      srv.MetricsHandler(),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  30 * time.Second,
		}
		go func() {
			logger.Printf("metrics listening on %s", cfg.Worker.MetricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Fatalf("metrics server failed: %v", err)
			}
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				logger.Printf("metrics shutdown failed: %v", err)
			}
		}()
	}

	if err := srv.Run(); err != nil {
		logger.Fatalf("worker failed: %v", err)
	}
}
