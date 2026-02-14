package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dunamismax/pixelflow/internal/domain"
	_ "github.com/lib/pq"
)

const jobSchemaSQL = `
CREATE TABLE IF NOT EXISTS jobs (
	id TEXT PRIMARY KEY,
	status TEXT NOT NULL,
	source_type TEXT NOT NULL,
	webhook_url TEXT NOT NULL DEFAULT '',
	pipeline JSONB NOT NULL,
	object_key TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);
`

type PostgresJobStore struct {
	db *sql.DB
}

func NewPostgresJobStore(ctx context.Context, dsn string) (*PostgresJobStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres connection: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &PostgresJobStore{db: db}
	if err := store.EnsureSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *PostgresJobStore) EnsureSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, jobSchemaSQL); err != nil {
		return fmt.Errorf("ensure jobs schema: %w", err)
	}
	return nil
}

func (s *PostgresJobStore) Close() error {
	return s.db.Close()
}

func (s *PostgresJobStore) Create(ctx context.Context, job domain.Job) error {
	pipelineJSON, err := json.Marshal(job.Pipeline)
	if err != nil {
		return fmt.Errorf("marshal job pipeline: %w", err)
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO jobs (id, status, source_type, webhook_url, pipeline, object_key, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		job.ID,
		job.Status,
		job.SourceType,
		job.WebhookURL,
		pipelineJSON,
		job.ObjectKey,
		job.CreatedAt,
		job.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}

	return nil
}

func (s *PostgresJobStore) Get(ctx context.Context, id string) (domain.Job, bool, error) {
	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, status, source_type, webhook_url, pipeline, object_key, created_at, updated_at
		 FROM jobs
		 WHERE id = $1`,
		id,
	)

	var (
		job          domain.Job
		pipelineJSON []byte
	)
	if err := row.Scan(
		&job.ID,
		&job.Status,
		&job.SourceType,
		&job.WebhookURL,
		&pipelineJSON,
		&job.ObjectKey,
		&job.CreatedAt,
		&job.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return domain.Job{}, false, nil
		}
		return domain.Job{}, false, fmt.Errorf("query job: %w", err)
	}

	if err := json.Unmarshal(pipelineJSON, &job.Pipeline); err != nil {
		return domain.Job{}, false, fmt.Errorf("unmarshal job pipeline: %w", err)
	}

	return job, true, nil
}

func (s *PostgresJobStore) UpdateStatus(ctx context.Context, id, status string) (domain.Job, error) {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE jobs
		 SET status = $1, updated_at = $2
		 WHERE id = $3`,
		status,
		now,
		id,
	)
	if err != nil {
		return domain.Job{}, fmt.Errorf("update job status: %w", err)
	}

	job, ok, err := s.Get(ctx, id)
	if err != nil {
		return domain.Job{}, err
	}
	if !ok {
		return domain.Job{}, ErrJobNotFound
	}

	return job, nil
}
