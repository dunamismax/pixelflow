package store

import (
	"context"

	"github.com/dunamismax/pixelflow/internal/domain"
)

type JobStore interface {
	Create(ctx context.Context, job domain.Job) error
	Get(ctx context.Context, id string) (domain.Job, bool, error)
	UpdateStatus(ctx context.Context, id, status string) (domain.Job, error)
}
