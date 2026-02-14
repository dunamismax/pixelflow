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

## Phase 2 (Completed): Image Pipeline (`govips` + local file mode)

- Worker pipeline package added with explicit stages:
  - input fetch
  - transform execution
  - output encode/write
- Worker lifecycle now initializes and shuts down `govips` when built with `-tags govips`.
- Implemented action handlers:
  - resize
  - watermark (text)
- Added local integration test for file-in -> transform -> file-out.
- Replaced `build/Dockerfile.worker-vips` scaffold with multi-stage CGO/libvips build.
- Scope boundary: object storage fetch/upload remains in Phase 3.

## Phase 3 (Completed): Object Storage + Presigned Flow

- Add MinIO/S3 client package.
- `POST /v1/jobs` should generate real presigned PUT URL.
- Add source-object existence checks before enqueueing.
- Add webhook callback with signed payload + retry/backoff.
- Replace in-memory API job store with Postgres-backed persistence.

## Phase 4 (Completed): Production Polish

- Added Redis-backed token bucket rate limiting middleware in API.
- Added `usage_logs` persistence in Postgres:
  - `user_id`
  - `job_id`
  - `pixels_processed`
  - `bytes_saved`
  - `compute_time_ms`
- Added Prometheus metrics endpoints for API and worker.
- Added OpenTelemetry tracing instrumentation for API requests and worker job processing.
- Published repeatable benchmark command and baseline results in `README.md`.

## Phase 5 (Planned): Reliability And Identity Hardening

- Replace header-derived `user_id` with authenticated identity propagation.
- Add webhook idempotency guard so callback retries do not emit duplicate downstream side effects.
