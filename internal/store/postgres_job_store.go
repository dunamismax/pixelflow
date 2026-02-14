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
	user_id TEXT NOT NULL DEFAULT 'anonymous',
	status TEXT NOT NULL,
	source_type TEXT NOT NULL,
	webhook_url TEXT NOT NULL DEFAULT '',
	pipeline JSONB NOT NULL,
	object_key TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);

ALTER TABLE jobs
ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT 'anonymous';
`

const usageLogSchemaSQL = `
CREATE TABLE IF NOT EXISTS usage_logs (
	job_id TEXT PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
	user_id TEXT NOT NULL,
	pixels_processed BIGINT NOT NULL,
	bytes_saved BIGINT NOT NULL,
	compute_time_ms BIGINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS usage_logs_user_id_created_at_idx
ON usage_logs (user_id, created_at DESC);
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
	if _, err := s.db.ExecContext(ctx, usageLogSchemaSQL); err != nil {
		return fmt.Errorf("ensure usage logs schema: %w", err)
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
		`INSERT INTO jobs (id, user_id, status, source_type, webhook_url, pipeline, object_key, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		job.ID,
		job.UserID,
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
		`SELECT id, user_id, status, source_type, webhook_url, pipeline, object_key, created_at, updated_at
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
		&job.UserID,
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

func (s *PostgresJobStore) CreateUsageLog(ctx context.Context, usage domain.UsageLog) error {
	createdAt := usage.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO usage_logs (job_id, user_id, pixels_processed, bytes_saved, compute_time_ms, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (job_id) DO UPDATE
		 SET user_id = EXCLUDED.user_id,
		     pixels_processed = EXCLUDED.pixels_processed,
		     bytes_saved = EXCLUDED.bytes_saved,
		     compute_time_ms = EXCLUDED.compute_time_ms,
		     created_at = EXCLUDED.created_at`,
		usage.JobID,
		usage.UserID,
		usage.PixelsProcessed,
		usage.BytesSaved,
		usage.ComputeTimeMS,
		createdAt,
	)
	if err != nil {
		return fmt.Errorf("insert usage log: %w", err)
	}

	return nil
}
