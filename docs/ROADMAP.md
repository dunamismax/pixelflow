# PixelFlow Delivery Roadmap

## Guiding Constraints

- Keep control plane light: no binary file streaming through API.
- Keep data plane isolated: heavy CGO/image work only in worker processes.
- Keep queue contracts explicit and versionable.
- Keep throughput stable: bound active image jobs with a semaphore.

## Phase 1 (Completed): Walking Skeleton

- Repo scaffolding with `cmd/`, `internal/`, `pkg/`.
- Local infra via `docker-compose.yml` (Redis, Postgres, MinIO).
- API endpoints:
  - `POST /v1/jobs` to validate and create a job contract.
  - `POST /v1/jobs/{id}/start` to enqueue Asynq task payload.
- Worker consumes Asynq tasks and logs processing execution.
- Core tests for request and queue payload contracts.

## Phase 2: Image Pipeline (`govips`)

- Add worker pipeline package with explicit stages:
  - input fetch
  - transform execution
  - output encode/upload
- Initialize and shut down `govips` safely in worker process lifecycle.
- Implement action handlers:
  - resize
  - watermark (text)
- Add integration test for local file input/output.

## Phase 3: Object Storage + Presigned Flow

- Add MinIO/S3 client package.
- `POST /v1/jobs` should generate real presigned PUT URL.
- Add source-object existence checks before enqueueing.
- Add webhook callback with signed payload + retry/backoff.

## Phase 4: Production Polish

- Add Redis-backed rate limiting middleware in API.
- Add `usage_logs` persistence in Postgres:
  - `user_id`
  - `job_id`
  - `pixels_processed`
  - `bytes_saved`
  - `compute_time_ms`
- Add Prometheus/OpenTelemetry metrics and tracing.
- Publish benchmark results in `README.md`.

