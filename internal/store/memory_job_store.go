package store

import (
	"errors"
	"sync"
	"time"

	"github.com/dunamismax/pixelflow/internal/domain"
)

var ErrJobNotFound = errors.New("job not found")

type MemoryJobStore struct {
	mu   sync.RWMutex
	jobs map[string]domain.Job
}

func NewMemoryJobStore() *MemoryJobStore {
	return &MemoryJobStore{
		jobs: make(map[string]domain.Job),
	}
}

func (s *MemoryJobStore) Create(job domain.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

func (s *MemoryJobStore) Get(id string) (domain.Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

func (s *MemoryJobStore) UpdateStatus(id, status string) (domain.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, ok := s.jobs[id]
	if !ok {
		return domain.Job{}, ErrJobNotFound
	}

	job.Status = status
	job.UpdatedAt = time.Now().UTC()
	s.jobs[id] = job
	return job, nil
}
