package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dunamismax/pixelflow/internal/config"
	"github.com/dunamismax/pixelflow/internal/domain"
	"github.com/dunamismax/pixelflow/internal/pipeline"
	"github.com/dunamismax/pixelflow/internal/queue"
	"github.com/dunamismax/pixelflow/internal/storage"
	"github.com/dunamismax/pixelflow/internal/store"
	"github.com/dunamismax/pixelflow/internal/webhook"
	"github.com/hibiken/asynq"
)

type Server struct {
	logger          *log.Logger
	server          *asynq.Server
	sem             chan struct{}
	localProcessor  *pipeline.Processor
	objectProcessor *pipeline.Processor
	webhookClient   webhookSender
	jobStore        store.JobStore
}

type webhookSender interface {
	Send(ctx context.Context, endpoint, event string, payload any) error
}

func NewServer(
	logger *log.Logger,
	queueCfg config.QueueConfig,
	workerCfg config.WorkerConfig,
	storageClient *storage.Client,
	webhookClient *webhook.Client,
	jobStore store.JobStore,
) (*Server, error) {
	if storageClient == nil {
		return nil, fmt.Errorf("storage client is required")
	}

	localProcessor, err := pipeline.NewLocalProcessor(workerCfg.LocalOutputDir)
	if err != nil {
		return nil, fmt.Errorf("initialize pipeline processor: %w", err)
	}

	objectProcessor, err := pipeline.NewObjectStoreProcessor(
		pipeline.ObjectStoreFetcher{Storage: storageClient},
		pipeline.ObjectStoreEmitter{Storage: storageClient, OutputPrefix: "outputs"},
	)
	if err != nil {
		return nil, fmt.Errorf("initialize object-store processor: %w", err)
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
		sem:             make(chan struct{}, max(1, workerCfg.MaxActiveJobs)),
		localProcessor:  localProcessor,
		objectProcessor: objectProcessor,
		webhookClient:   webhookClient,
		jobStore:        jobStore,
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

	s.updateJobStatus(ctx, payload.JobID, domain.JobStatusProcessing)

	request := pipeline.Request{
		JobID:      payload.JobID,
		SourceType: payload.SourceType,
		ObjectKey:  payload.ObjectKey,
		Pipeline:   payload.Pipeline,
	}

	var result pipeline.Result
	switch payload.SourceType {
	case domain.SourceTypeLocalFile:
		result, err = s.localProcessor.Process(ctx, request)
	default:
		result, err = s.objectProcessor.Process(ctx, request)
	}
	if err != nil {
		s.updateJobStatus(ctx, payload.JobID, domain.JobStatusFailed)
		s.dispatchWebhook(ctx, payload, "job.failed", map[string]any{
			"job_id":       payload.JobID,
			"status":       domain.JobStatusFailed,
			"source_type":  payload.SourceType,
			"object_key":   payload.ObjectKey,
			"requested_at": payload.RequestedAt,
			"failed_at":    time.Now().UTC(),
			"error":        err.Error(),
		})
		return fmt.Errorf("run pipeline: %w", err)
	}

	s.logger.Printf("Processed job_id=%s outputs=%d", payload.JobID, len(result.Outputs))
	s.updateJobStatus(ctx, payload.JobID, domain.JobStatusSucceeded)

	if err := s.dispatchWebhook(ctx, payload, "job.completed", map[string]any{
		"job_id":       payload.JobID,
		"status":       domain.JobStatusSucceeded,
		"source_type":  payload.SourceType,
		"object_key":   payload.ObjectKey,
		"requested_at": payload.RequestedAt,
		"completed_at": time.Now().UTC(),
		"outputs":      result.Outputs,
	}); err != nil {
		return err
	}

	return nil
}

func (s *Server) updateJobStatus(ctx context.Context, jobID, status string) {
	if s.jobStore == nil {
		return
	}
	if _, err := s.jobStore.UpdateStatus(ctx, jobID, status); err != nil {
		s.logger.Printf("job status update failed job_id=%s status=%s err=%v", jobID, status, err)
	}
}

func (s *Server) dispatchWebhook(ctx context.Context, payload queue.ProcessImagePayload, event string, body map[string]any) error {
	if payload.WebhookURL == "" || s.webhookClient == nil {
		return nil
	}

	if err := s.webhookClient.Send(ctx, payload.WebhookURL, event, body); err != nil {
		s.logger.Printf("webhook delivery failed job_id=%s event=%s err=%v", payload.JobID, event, err)
		return fmt.Errorf("dispatch webhook: %w", err)
	}

	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
