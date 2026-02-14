package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/dunamismax/pixelflow/internal/domain"
	"github.com/dunamismax/pixelflow/internal/id"
	"github.com/dunamismax/pixelflow/internal/queue"
	"github.com/dunamismax/pixelflow/internal/store"
)

type Server struct {
	logger      *log.Logger
	queueClient *queue.Client
	jobStore    *store.MemoryJobStore
	mux         *http.ServeMux
}

func NewServer(logger *log.Logger, queueClient *queue.Client, jobStore *store.MemoryJobStore) *Server {
	s := &Server{
		logger:      logger,
		queueClient: queueClient,
		jobStore:    jobStore,
		mux:         http.NewServeMux(),
	}
	s.routes()
	return s
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
	job := domain.Job{
		ID:         jobID,
		Status:     domain.JobStatusCreated,
		SourceType: req.SourceType,
		WebhookURL: req.WebhookURL,
		Pipeline:   req.Pipeline,
		ObjectKey:  fmt.Sprintf("uploads/%s/source", jobID),
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	s.jobStore.Create(job)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id": job.ID,
		"status": job.Status,
		"upload": map[string]string{
			"object_key":          job.ObjectKey,
			"presigned_put_url":   "",
			"presigned_url_state": "pending_phase_3",
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

	job, ok := s.jobStore.Get(jobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
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

	if _, err := s.jobStore.UpdateStatus(job.ID, domain.JobStatusQueued); err != nil {
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
