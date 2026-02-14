package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dunamismax/pixelflow/internal/domain"
	"github.com/dunamismax/pixelflow/internal/id"
	"github.com/dunamismax/pixelflow/internal/queue"
	"github.com/dunamismax/pixelflow/internal/store"
	"github.com/hibiken/asynq"
)

type Server struct {
	logger      *log.Logger
	queueClient queueEnqueuer
	jobStore    store.JobStore
	storage     objectStorage
	presignTTL  time.Duration
	mux         *http.ServeMux
}

type queueEnqueuer interface {
	EnqueueProcessImage(ctx context.Context, payload queue.ProcessImagePayload) (*asynq.TaskInfo, error)
}

type objectStorage interface {
	PresignedPutURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	ObjectExists(ctx context.Context, objectKey string) (bool, error)
}

func NewServer(logger *log.Logger, queueClient queueEnqueuer, jobStore store.JobStore, storage objectStorage, presignTTL time.Duration) *Server {
	if presignTTL <= 0 {
		presignTTL = 15 * time.Minute
	}
	if storage == nil {
		storage = unavailableObjectStorage{}
	}

	s := &Server{
		logger:      logger,
		queueClient: queueClient,
		jobStore:    jobStore,
		storage:     storage,
		presignTTL:  presignTTL,
		mux:         http.NewServeMux(),
	}
	s.routes()
	return s
}

type unavailableObjectStorage struct{}

func (unavailableObjectStorage) PresignedPutURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("object storage is unavailable")
}

func (unavailableObjectStorage) ObjectExists(_ context.Context, _ string) (bool, error) {
	return false, errors.New("object storage is unavailable")
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("POST /v1/jobs", s.handleCreateJob)
	s.mux.HandleFunc("POST /v1/jobs/", s.handleStartJob)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateJobRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := req.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	now := time.Now().UTC()
	jobID := id.New()
	sourceType := strings.ToLower(strings.TrimSpace(req.SourceType))
	objectKey := strings.TrimSpace(req.ObjectKey)
	uploadState := "not_required"
	presignedPutURL := ""

	if sourceType == domain.SourceTypeS3Presigned {
		objectKey = fmt.Sprintf("uploads/%s/source", jobID)
		url, err := s.storage.PresignedPutURL(r.Context(), objectKey, s.presignTTL)
		if err != nil {
			s.logger.Printf("generate presigned url failed for job %s: %v", jobID, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate upload URL"})
			return
		}
		presignedPutURL = url
		uploadState = "ready"
	}

	job := domain.Job{
		ID:         jobID,
		Status:     domain.JobStatusCreated,
		SourceType: sourceType,
		WebhookURL: req.WebhookURL,
		Pipeline:   req.Pipeline,
		ObjectKey:  objectKey,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if err := s.jobStore.Create(r.Context(), job); err != nil {
		s.logger.Printf("create job failed for job %s: %v", job.ID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create job"})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id": job.ID,
		"status": job.Status,
		"upload": map[string]string{
			"object_key":          job.ObjectKey,
			"presigned_put_url":   presignedPutURL,
			"presigned_url_state": uploadState,
		},
		"start_url": fmt.Sprintf("/v1/jobs/%s/start", job.ID),
	})
}

func (s *Server) handleStartJob(w http.ResponseWriter, r *http.Request) {
	jobID, err := extractJobIDFromStartPath(r.URL.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	job, ok, err := s.jobStore.Get(r.Context(), jobID)
	if err != nil {
		s.logger.Printf("fetch job failed for job %s: %v", jobID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load job"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}

	if err := s.verifySourceExists(r.Context(), job); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}

	payload := queue.ProcessImagePayload{
		JobID:       job.ID,
		SourceType:  job.SourceType,
		WebhookURL:  job.WebhookURL,
		ObjectKey:   job.ObjectKey,
		Pipeline:    job.Pipeline,
		RequestedAt: time.Now().UTC(),
	}

	taskInfo, err := s.queueClient.EnqueueProcessImage(r.Context(), payload)
	if err != nil {
		s.logger.Printf("enqueue failed for job %s: %v", job.ID, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to enqueue job"})
		return
	}

	if _, err := s.jobStore.UpdateStatus(r.Context(), job.ID, domain.JobStatusQueued); err != nil {
		s.logger.Printf("update status failed for job %s: %v", job.ID, err)
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":      job.ID,
		"status":      domain.JobStatusQueued,
		"queue":       taskInfo.Queue,
		"task_id":     taskInfo.ID,
		"state":       taskInfo.State.String(),
		"enqueued_at": taskInfo.NextProcessAt,
	})
}

func (s *Server) verifySourceExists(ctx context.Context, job domain.Job) error {
	switch job.SourceType {
	case domain.SourceTypeLocalFile:
		if _, err := os.Stat(job.ObjectKey); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("source object is missing: %s", job.ObjectKey)
			}
			return fmt.Errorf("source object check failed: %w", err)
		}
		return nil
	default:
		exists, err := s.storage.ObjectExists(ctx, job.ObjectKey)
		if err != nil {
			return fmt.Errorf("source object check failed: %w", err)
		}
		if !exists {
			return fmt.Errorf("source object is missing: %s", job.ObjectKey)
		}
		return nil
	}
}

func extractJobIDFromStartPath(path string) (string, error) {
	trimmed := strings.TrimPrefix(path, "/v1/jobs/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "start" {
		return "", errors.New("expected path format /v1/jobs/{id}/start")
	}
	return parts[0], nil
}

func decodeJSON(r *http.Request, into any) error {
	const maxBodyBytes = 1 << 20
	limited := io.LimitReader(r.Body, maxBodyBytes)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid JSON body: multiple JSON values are not allowed")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
