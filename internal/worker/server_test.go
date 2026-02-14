package worker

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/dunamismax/pixelflow/internal/domain"
	"github.com/dunamismax/pixelflow/internal/pipeline"
	"github.com/dunamismax/pixelflow/internal/store"
)

func TestRecordUsageWritesUsageLog(t *testing.T) {
	jobStore := store.NewMemoryJobStore()
	if err := jobStore.Create(context.Background(), domain.Job{
		ID:         "job-1",
		UserID:     "user-1",
		Status:     domain.JobStatusProcessing,
		SourceType: domain.SourceTypeLocalFile,
		ObjectKey:  "input.png",
		Pipeline:   []domain.PipelineStep{{ID: "thumb", Action: "resize", Width: 100}},
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	usageStore := &captureUsageStore{}
	s := &Server{
		logger:     log.New(io.Discard, "", 0),
		jobStore:   jobStore,
		usageStore: usageStore,
		metrics:    newMetrics(),
	}

	s.recordUsage(context.Background(), "job-1", pipeline.Result{
		SourceBytes: 1_000,
		Outputs: []pipeline.Output{
			{Width: 10, Height: 10, Bytes: 300},
			{Width: 20, Height: 20, Bytes: 400},
		},
	}, 250*time.Millisecond)

	if !usageStore.called {
		t.Fatal("expected usage log to be written")
	}
	if usageStore.log.UserID != "user-1" {
		t.Fatalf("expected user_id=user-1, got %s", usageStore.log.UserID)
	}
	if usageStore.log.PixelsProcessed != 500 {
		t.Fatalf("expected pixels_processed=500, got %d", usageStore.log.PixelsProcessed)
	}
	if usageStore.log.BytesSaved != 300 {
		t.Fatalf("expected bytes_saved=300, got %d", usageStore.log.BytesSaved)
	}
	if usageStore.log.ComputeTimeMS != 250 {
		t.Fatalf("expected compute_time_ms=250, got %d", usageStore.log.ComputeTimeMS)
	}
}

func TestRecordUsageClampsNegativeBytesSaved(t *testing.T) {
	usageStore := &captureUsageStore{}
	s := &Server{
		logger:     log.New(io.Discard, "", 0),
		usageStore: usageStore,
		metrics:    newMetrics(),
	}

	s.recordUsage(context.Background(), "job-2", pipeline.Result{
		SourceBytes: 100,
		Outputs: []pipeline.Output{
			{Width: 5, Height: 5, Bytes: 200},
		},
	}, 0)

	if usageStore.log.BytesSaved != 0 {
		t.Fatalf("expected bytes_saved=0, got %d", usageStore.log.BytesSaved)
	}
	if usageStore.log.ComputeTimeMS < 1 {
		t.Fatalf("expected compute_time_ms to be at least 1, got %d", usageStore.log.ComputeTimeMS)
	}
}

type captureUsageStore struct {
	called bool
	log    domain.UsageLog
}

func (s *captureUsageStore) CreateUsageLog(_ context.Context, usage domain.UsageLog) error {
	s.called = true
	s.log = usage
	return nil
}
