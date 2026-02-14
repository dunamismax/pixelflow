package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dunamismax/pixelflow/internal/api"
	"github.com/dunamismax/pixelflow/internal/config"
	"github.com/dunamismax/pixelflow/internal/queue"
	"github.com/dunamismax/pixelflow/internal/ratelimit"
	"github.com/dunamismax/pixelflow/internal/storage"
	"github.com/dunamismax/pixelflow/internal/store"
	"github.com/dunamismax/pixelflow/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "[api] ", log.LstdFlags|log.Lmsgprefix)

	traceShutdown, err := telemetry.SetupTracing(context.Background(), telemetry.TraceConfig{
		ServiceName:  "pixelflow-api",
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

	queueClient := queue.NewClient(cfg.Queue.RedisClientOpt(), cfg.Queue.Name)
	defer func() {
		if err := queueClient.Close(); err != nil {
			logger.Printf("queue client close error: %v", err)
		}
	}()

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

	jobStore, err := store.NewPostgresJobStore(startupCtx, cfg.Database.DSN)
	if err != nil {
		logger.Fatalf("job store init failed: %v", err)
	}
	defer func() {
		if err := jobStore.Close(); err != nil {
			logger.Printf("job store close error: %v", err)
		}
	}()

	serverOpts := []api.Option{
		api.WithRateLimiter(nil, cfg.API.RateLimitUserID),
	}
	if cfg.API.RateLimitEnabled {
		redisClient := redis.NewClient(&redis.Options{
			Addr:     cfg.Queue.RedisAddr,
			Password: cfg.Queue.RedisPassword,
			DB:       cfg.Queue.RedisDB,
		})
		if err := redisClient.Ping(startupCtx).Err(); err != nil {
			logger.Fatalf("rate limiter redis ping failed: %v", err)
		}
		defer func() {
			if err := redisClient.Close(); err != nil {
				logger.Printf("rate limiter redis close error: %v", err)
			}
		}()

		limiter, err := ratelimit.NewRedisTokenBucket(
			redisClient,
			cfg.API.RateLimitCapacity,
			cfg.API.RateLimitWindow,
			"pixelflow:api:ratelimit",
		)
		if err != nil {
			logger.Fatalf("rate limiter init failed: %v", err)
		}
		serverOpts = append(serverOpts, api.WithRateLimiter(limiter, cfg.API.RateLimitUserID))
	}

	app := api.NewServer(logger, queueClient, jobStore, storageClient, cfg.Storage.PresignPutExpiry, serverOpts...)

	httpServer := &http.Server{
		Addr:         cfg.API.Addr,
		Handler:      app.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Printf("listening on %s", cfg.API.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	var metricsServer *http.Server
	if strings.TrimSpace(cfg.API.MetricsAddr) != "" {
		metricsServer = &http.Server{
			Addr:         cfg.API.MetricsAddr,
			Handler:      app.MetricsHandler(),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  30 * time.Second,
		}
		go func() {
			logger.Printf("metrics listening on %s", cfg.API.MetricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Fatalf("metrics server failed: %v", err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Println("shutting down")
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Printf("graceful shutdown failed: %v", err)
	}
	if metricsServer != nil {
		if err := metricsServer.Shutdown(ctx); err != nil {
			logger.Printf("metrics shutdown failed: %v", err)
		}
	}
}
