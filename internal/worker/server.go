package worker

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dunamismax/pixelflow/internal/config"
	"github.com/dunamismax/pixelflow/internal/domain"
	"github.com/dunamismax/pixelflow/internal/pipeline"
	"github.com/dunamismax/pixelflow/internal/queue"
	"github.com/dunamismax/pixelflow/internal/storage"
	"github.com/dunamismax/pixelflow/internal/store"
	"github.com/dunamismax/pixelflow/internal/webhook"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Server struct {
	logger          *log.Logger
	server          *asynq.Server
	sem             chan struct{}
	localProcessor  *pipeline.Processor
	objectProcessor *pipeline.Processor
	webhookClient   webhookSender
	jobStore        store.JobStore
	usageStore      store.UsageStore
	metrics         *metrics
	tracer          trace.Tracer
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
	usageStore store.UsageStore,
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

	if usageStore == nil {
		if jobAndUsageStore, ok := jobStore.(store.UsageStore); ok {
			usageStore = jobAndUsageStore
		}
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
		usageStore:      usageStore,
		metrics:         newMetrics(),
		tracer:          otel.Tracer("pixelflow/worker"),
	}
	return s, nil
}

func (s *Server) Run() error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(queue.TypeProcessImage, s.handleProcessImage)
	return s.server.Run(mux)
}

func (s *Server) MetricsHandler() http.Handler {
	return s.metrics.Handler()
}

func (s *Server) handleProcessImage(ctx context.Context, task *asynq.Task) error {
	startedAt := time.Now()
	outcome := domain.JobStatusFailed

	payload, err := queue.ParseProcessImagePayload(task)
	if err != nil {
		return fmt.Errorf("parse payload: %v: %w", err, asynq.SkipRetry)
	}

	ctx, span := s.tracer.Start(ctx, "worker.process_image", trace.WithSpanKind(trace.SpanKindConsumer))
	span.SetAttributes(
		attribute.String("job.id", payload.JobID),
		attribute.String("job.source_type", payload.SourceType),
		attribute.Int("job.pipeline_steps", len(payload.Pipeline)),
	)
	defer span.End()
	defer func() {
		s.metrics.jobDuration.WithLabelValues(payload.SourceType, outcome).Observe(time.Since(startedAt).Seconds())
		s.metrics.jobsTotal.WithLabelValues(payload.SourceType, outcome).Inc()
	}()

	s.sem <- struct{}{}
	s.metrics.activeJobs.Inc()
	defer func() {
		<-s.sem
		s.metrics.activeJobs.Dec()
	}()

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
		span.RecordError(err)
		span.SetStatus(codes.Error, "pipeline failed")
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
	s.metrics.pipelineOutputsTotal.Add(float64(len(result.Outputs)))
	s.recordUsage(ctx, payload.JobID, result, time.Since(startedAt))

	if err := s.dispatchWebhook(ctx, payload, "job.completed", map[string]any{
		"job_id":       payload.JobID,
		"status":       domain.JobStatusSucceeded,
		"source_type":  payload.SourceType,
		"object_key":   payload.ObjectKey,
		"requested_at": payload.RequestedAt,
		"completed_at": time.Now().UTC(),
		"outputs":      result.Outputs,
	}); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "webhook dispatch failed")
		return err
	}

	outcome = domain.JobStatusSucceeded
	span.SetStatus(codes.Ok, "processed")
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

func (s *Server) recordUsage(ctx context.Context, jobID string, result pipeline.Result, computeDuration time.Duration) {
	if s.usageStore == nil {
		return
	}

	userID := "anonymous"
	if s.jobStore != nil {
		job, ok, err := s.jobStore.Get(ctx, jobID)
		if err != nil {
			s.logger.Printf("usage lookup failed job_id=%s err=%v", jobID, err)
		} else if ok && strings.TrimSpace(job.UserID) != "" {
			userID = job.UserID
		}
	}

	var (
		pixelsProcessed  int64
		totalOutputBytes int
	)
	for _, output := range result.Outputs {
		pixelsProcessed += int64(output.Width * output.Height)
		totalOutputBytes += output.Bytes
	}

	bytesSaved := int64(result.SourceBytes - totalOutputBytes)
	if bytesSaved < 0 {
		bytesSaved = 0
	}

	computeTimeMS := computeDuration.Milliseconds()
	if computeTimeMS < 1 {
		computeTimeMS = 1
	}

	usage := domain.UsageLog{
		UserID:          userID,
		JobID:           jobID,
		PixelsProcessed: pixelsProcessed,
		BytesSaved:      bytesSaved,
		ComputeTimeMS:   computeTimeMS,
		CreatedAt:       time.Now().UTC(),
	}
	if err := s.usageStore.CreateUsageLog(ctx, usage); err != nil {
		s.logger.Printf("usage log write failed job_id=%s err=%v", jobID, err)
		return
	}

	s.metrics.pixelsProcessedTotal.Add(float64(pixelsProcessed))
	s.metrics.bytesSavedTotal.Add(float64(bytesSaved))
	s.metrics.computeTimeMSTotal.Add(float64(computeTimeMS))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
