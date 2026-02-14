package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dunamismax/pixelflow/internal/config"
	"github.com/dunamismax/pixelflow/internal/queue"
	"github.com/hibiken/asynq"
)

type Server struct {
	logger *log.Logger
	server *asynq.Server
	sem    chan struct{}
}

func NewServer(logger *log.Logger, queueCfg config.QueueConfig, workerCfg config.WorkerConfig) *Server {
	s := &Server{
		logger: logger,
		server: asynq.NewServer(
			queueCfg.RedisClientOpt(),
			asynq.Config{
				Concurrency: workerCfg.Concurrency,
				Queues: map[string]int{
					queueCfg.Name: 1,
				},
				LogLevel: asynq.InfoLevel,
				ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
					retried, _ := asynq.GetRetryCount(ctx)
					maxRetry, _ := asynq.GetMaxRetry(ctx)
					logger.Printf("task failed type=%s retry=%d/%d err=%v", task.Type(), retried, maxRetry, err)
				}),
			},
		),
		sem: make(chan struct{}, max(1, workerCfg.MaxActiveJobs)),
	}
	return s
}

func (s *Server) Run() error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TypeProcessImage, s.handleProcessImage)
	return s.server.Run(mux)
}

func (s *Server) handleProcessImage(ctx context.Context, task *asynq.Task) error {
	payload, err := queue.ParseProcessImagePayload(task)
	if err != nil {
		return fmt.Errorf("parse payload: %v: %w", err, asynq.SkipRetry)
	}

	s.sem <- struct{}{}
	defer func() { <-s.sem }()

	s.logger.Printf(
		"Working... job_id=%s source_type=%s outputs=%d object_key=%s",
		payload.JobID,
		payload.SourceType,
		len(payload.Pipeline),
		payload.ObjectKey,
	)

	// Placeholder for the real image pipeline (govips + S3 I/O in Phase 2/3).
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(600 * time.Millisecond):
	}

	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
