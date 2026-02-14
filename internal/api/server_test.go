package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dunamismax/pixelflow/internal/domain"
	"github.com/dunamismax/pixelflow/internal/queue"
	"github.com/dunamismax/pixelflow/internal/ratelimit"
	"github.com/dunamismax/pixelflow/internal/store"
	"github.com/hibiken/asynq"
)

func TestExtractJobIDFromStartPath(t *testing.T) {
	jobID, err := extractJobIDFromStartPath("/v1/jobs/abc123/start")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if jobID != "abc123" {
		t.Fatalf("expected abc123, got %s", jobID)
	}

	if _, err := extractJobIDFromStartPath("/v1/jobs/abc123"); err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestCreateJobReturnsPresignedURLForS3Source(t *testing.T) {
	jobStore := store.NewMemoryJobStore()
	storageClient := &fakeStorage{
		presignedURL: "http://minio.local/presigned-put",
	}
	server := NewServer(
		testLogger(t),
		&fakeQueueClient{},
		jobStore,
		storageClient,
		15*time.Minute,
	)

	reqBody := `{
		"source_type":"s3_presigned",
		"pipeline":[{"id":"thumb","action":"resize","width":120}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	upload, ok := body["upload"].(map[string]any)
	if !ok {
		t.Fatalf("expected upload payload in response")
	}

	if got := upload["presigned_url_state"]; got != "ready" {
		t.Fatalf("expected presigned_url_state=ready, got %v", got)
	}
	if got := upload["presigned_put_url"]; got != "http://minio.local/presigned-put" {
		t.Fatalf("expected presigned URL in response, got %v", got)
	}
}

func TestStartJobRejectsMissingSourceObject(t *testing.T) {
	jobStore := store.NewMemoryJobStore()
	if err := jobStore.Create(context.Background(), domain.Job{
		ID:         "job-1",
		Status:     domain.JobStatusCreated,
		SourceType: domain.SourceTypeS3Presigned,
		ObjectKey:  "uploads/job-1/source",
		Pipeline: []domain.PipelineStep{
			{ID: "thumb", Action: "resize", Width: 100},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create seed job: %v", err)
	}

	queueClient := &fakeQueueClient{}
	server := NewServer(
		testLogger(t),
		queueClient,
		jobStore,
		&fakeStorage{exists: false},
		15*time.Minute,
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-1/start", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}
	if queueClient.called {
		t.Fatal("expected enqueue to be skipped when source object is missing")
	}
}

func TestCreateJobPersistsAnonymousUserIDByDefault(t *testing.T) {
	jobStore := store.NewMemoryJobStore()
	server := NewServer(
		testLogger(t),
		&fakeQueueClient{},
		jobStore,
		&fakeStorage{presignedURL: "http://minio.local/presigned-put"},
		15*time.Minute,
	)

	reqBody := `{
		"source_type":"s3_presigned",
		"pipeline":[{"id":"thumb","action":"resize","width":120}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	jobID, ok := body["job_id"].(string)
	if !ok || jobID == "" {
		t.Fatalf("expected job_id string, got %v", body["job_id"])
	}

	job, found, err := jobStore.Get(context.Background(), jobID)
	if err != nil {
		t.Fatalf("fetch job: %v", err)
	}
	if !found {
		t.Fatal("expected job to be persisted")
	}
	if job.UserID != "anonymous" {
		t.Fatalf("expected user_id=anonymous, got %s", job.UserID)
	}
}

func TestRateLimitMiddlewareRejectsWhenBucketDenied(t *testing.T) {
	jobStore := store.NewMemoryJobStore()
	server := NewServer(
		testLogger(t),
		&fakeQueueClient{},
		jobStore,
		&fakeStorage{presignedURL: "http://minio.local/presigned-put"},
		15*time.Minute,
		WithRateLimiter(&fakeRateLimiter{
			decision: ratelimit.Decision{Allowed: false, Remaining: 0, RetryAfter: 2 * time.Second},
		}, "X-User-ID"),
	)

	reqBody := `{
		"source_type":"s3_presigned",
		"pipeline":[{"id":"thumb","action":"resize","width":120}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "alice")

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("expected retry-after=2, got %s", got)
	}
}

type fakeQueueClient struct {
	called bool
}

func (f *fakeQueueClient) EnqueueProcessImage(_ context.Context, _ queue.ProcessImagePayload) (*asynq.TaskInfo, error) {
	f.called = true
	return &asynq.TaskInfo{
		ID:            "task-1",
		Queue:         "default",
		State:         asynq.TaskStateActive,
		NextProcessAt: time.Now().UTC(),
	}, nil
}

type fakeStorage struct {
	presignedURL string
	exists       bool
}

func (f *fakeStorage) PresignedPutURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return f.presignedURL, nil
}

func (f *fakeStorage) ObjectExists(_ context.Context, _ string) (bool, error) {
	return f.exists, nil
}

type fakeRateLimiter struct {
	decision ratelimit.Decision
	err      error
}

func (f *fakeRateLimiter) Allow(_ context.Context, _ string) (ratelimit.Decision, error) {
	return f.decision, f.err
}

func testLogger(t *testing.T) *log.Logger {
	t.Helper()
	return log.New(io.Discard, "", 0)
}
