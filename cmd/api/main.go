package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dunamismax/pixelflow/internal/api"
	"github.com/dunamismax/pixelflow/internal/config"
	"github.com/dunamismax/pixelflow/internal/queue"
	"github.com/dunamismax/pixelflow/internal/storage"
	"github.com/dunamismax/pixelflow/internal/store"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "[api] ", log.LstdFlags|log.Lmsgprefix)

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

	app := api.NewServer(logger, queueClient, jobStore, storageClient, cfg.Storage.PresignPutExpiry)

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

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	logger.Println("shutting down")
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Printf("graceful shutdown failed: %v", err)
	}
}
