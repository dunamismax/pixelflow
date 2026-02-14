package worker

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/dunamismax/pixelflow/internal/config"
	"github.com/dunamismax/pixelflow/internal/pipeline"
	"github.com/dunamismax/pixelflow/internal/queue"
	"github.com/hibiken/asynq"
)

type Server struct {
	logger    *log.Logger
	server    *asynq.Server
	sem       chan struct{}
	processor *pipeline.Processor
}

func NewServer(logger *log.Logger, queueCfg config.QueueConfig, workerCfg config.WorkerConfig) (*Server, error) {
	processor, err := pipeline.NewLocalProcessor(workerCfg.LocalOutputDir)
	if err != nil {
		return nil, fmt.Errorf("initialize pipeline processor: %w", err)
	}

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
		sem:       make(chan struct{}, max(1, workerCfg.MaxActiveJobs)),
		processor: processor,
	}
	return s, nil
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

	// Phase 2 supports local file I/O. S3/MinIO source fetch and upload stay in Phase 3.
	if strings.EqualFold(payload.SourceType, pipeline.SourceTypeLocalFile) {
		result, err := s.processor.Process(ctx, pipeline.Request{
			JobID:      payload.JobID,
			SourceType: payload.SourceType,
			ObjectKey:  payload.ObjectKey,
			Pipeline:   payload.Pipeline,
		})
		if err != nil {
			return fmt.Errorf("run local pipeline: %w", err)
		}

		s.logger.Printf("Processed job_id=%s outputs=%d", payload.JobID, len(result.Outputs))
		return nil
	}

	s.logger.Printf("deferred processing for job_id=%s source_type=%s (pending object storage flow in phase 3)", payload.JobID, payload.SourceType)
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
