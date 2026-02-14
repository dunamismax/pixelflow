package main

import (
	"log"
	"os"

	"github.com/dunamismax/pixelflow/internal/config"
	"github.com/dunamismax/pixelflow/internal/pipeline"
	"github.com/dunamismax/pixelflow/internal/worker"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "[worker] ", log.LstdFlags|log.Lmsgprefix)

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

	srv, err := worker.NewServer(logger, cfg.Queue, cfg.Worker)
	if err != nil {
		logger.Fatalf("worker init failed: %v", err)
	}
	if err := srv.Run(); err != nil {
		logger.Fatalf("worker failed: %v", err)
	}
}
