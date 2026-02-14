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

	jobStore := store.NewMemoryJobStore()
	app := api.NewServer(logger, queueClient, jobStore)

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
