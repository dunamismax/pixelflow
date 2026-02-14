# AGENTS.md

Repository guidance for AI coding agents working on `pixelflow`.

Last updated: 2026-02-14
Owner: `dunamismax`
Repo: `https://github.com/dunamismax/pixelflow`

## 1. Purpose

This file is the canonical, agent-focused operating guide for this repository.

Use it to:

- Understand the current architecture and implementation status quickly.
- Run the right build/test commands without trial-and-error.
- Apply project conventions consistently.
- Avoid risky or unwanted actions.
- Keep project roadmap and maintenance state current.

## 2. Instruction Priority And Scope

When instructions conflict, use this order:

1. Direct user request in current conversation.
2. Closest `AGENTS.md` in the edited file’s directory tree.
3. Parent/root `AGENTS.md`.
4. Other docs (`README.md`, `docs/ROADMAP.md`, comments).

If this repository gains nested `AGENTS.md` files later, the nearest one takes precedence for files in that subtree.

## 3. Hard Safety Rules

1. Never run `git commit`.
2. Never run `git push`.
3. Never create tags/releases on behalf of the user.
4. Never rewrite git history (`git reset --hard`, force-push, etc.) unless the user explicitly requests destructive recovery and confirms.
5. Never add secrets/tokens to code, docs, tests, logs, or example env files.
6. Never assume infra is running; verify or report what could not be verified.

If the user asks for code changes, produce patch-level changes and verification results. Let the user perform commit/push actions.

## 4. Project Snapshot (Current Reality)

Project name: **PixelFlow**
Tagline: **High-Throughput, Asynchronous Image Processing Pipeline**

Current implementation state:

1. Phase 1 walking skeleton: implemented.
2. Phase 2 local-file pipeline: implemented.
3. Phase 3 object-storage flow: implemented.
4. API endpoints:
   - `GET /healthz`
   - `POST /v1/jobs`
   - `POST /v1/jobs/{id}/start`
5. Queue worker:
   - Asynq task type: `image:process`
   - Uses explicit pipeline stages (`fetch`, `transform`, `emit`) for both `source_type=local_file` and `source_type=s3_presigned`.
   - Supports `resize` and text `watermark` actions.
   - Updates job status transitions (`processing`, `succeeded`, `failed`) in Postgres.
6. Concurrency guard:
   - Semaphore-based active-job limit exists in worker.
7. Storage/persistence:
   - MinIO/S3 client is implemented for presign/stat/get/put operations.
   - API job state is persisted in Postgres `jobs` table.
   - `POST /v1/jobs` returns real presigned PUT URLs for `s3_presigned` jobs.

## 5. Architecture Intent

1. Control Plane (`cmd/api`):
   - Authenticate/validate/create jobs.
   - Generate presigned upload URLs for object-storage uploads.
   - Enqueue work only after upload/start signal.
2. Data Plane (`cmd/worker`):
   - Pull queue tasks.
   - Run image processing via pipeline package.
   - `govips` runtime is enabled when built with `-tags govips`; default dev builds use stdlib fallback.
   - Upload outputs and send webhook callbacks (signed payload + retry/backoff).
3. Local infra:
   - Redis for queue.
   - Postgres for durable jobs/usage.
   - MinIO for S3-compatible object storage.

## 6. Key Files And Responsibilities

Core runtime:

- `cmd/api/main.go`: API process bootstrap and graceful shutdown.
- `cmd/worker/main.go`: worker process bootstrap.
- `internal/api/server.go`: route handlers and request/response behavior.
- `internal/worker/server.go`: Asynq worker config, task handling, semaphore control.
- `internal/queue/tasks.go`: task type and payload contract.
- `internal/queue/client.go`: enqueue behavior and options.
- `internal/pipeline/processor.go`: phase 2 fetch/transform/emit orchestration.
- `internal/pipeline/object_store_stages.go`: object-storage fetch + emit stages for `s3_presigned`.
- `internal/pipeline/transformer_std.go`: default resize + text watermark transformer.
- `internal/pipeline/transformer_govips.go`: `govips` transformer (build tag: `govips` + `cgo`).
- `internal/domain/job.go`: request and domain types.
- `internal/storage/client.go`: MinIO/S3 client wrapper for presign/stat/get/put.
- `internal/store/postgres_job_store.go`: Postgres-backed `jobs` persistence with schema bootstrap.
- `internal/store/memory_job_store.go`: in-memory store used in tests/fallback-only scenarios.
- `internal/webhook/client.go`: signed webhook sender with retry/backoff.
- `internal/config/config.go`: environment-driven configuration.

Infra/docs:

- `docker-compose.yml`: local Redis/Postgres/MinIO stack + MinIO bucket init.
- `.env.example`: env contract for local/dev setup.
- `Makefile`: common dev commands.
- `README.md`: human-facing quickstart and project overview.
- `docs/ROADMAP.md`: roadmap detail by phase.
- `build/Dockerfile.api`: API container build.
- `build/Dockerfile.worker`: worker container build (Phase 1 friendly).
- `build/Dockerfile.worker-vips`: multi-stage CGO/libvips worker build (with `-tags govips`).

## 7. Standard Commands (Use These First)

Environment:

1. `docker compose up -d`
2. `docker compose down`
3. `docker compose logs -f redis postgres minio`

Build/run:

1. `go run ./cmd/api`
2. `go run ./cmd/worker`
3. `docker build -f build/Dockerfile.worker-vips -t pixelflow-worker-vips .`

Quality checks:

1. `go mod tidy`
2. `gofmt -w <files>`
3. `go test ./...`
4. `go test ./internal/pipeline -run TestLocalProcessor_FileInTransformFileOut`

Make shortcuts:

1. `make up`
2. `make down`
3. `make logs`
4. `make run-api`
5. `make run-worker`
6. `make tidy`
7. `make test`

## 8. Coding Standards (Go)

1. Keep packages focused and small; avoid circular dependencies.
2. Use explicit domain types for queue/API contracts.
3. Return wrapped errors with useful context (`fmt.Errorf("...: %w", err)`).
4. Keep handlers strict on input validation and unknown JSON fields.
5. Keep control-plane handlers lightweight; no direct heavy image work in API.
6. Keep worker throughput stable; respect semaphore limit for active heavy jobs.
7. Preserve backwards compatibility for task payload fields when possible.
8. Add/update tests for behavior changed.

## 9. API And Queue Contracts (Current)

Current API:

1. `POST /v1/jobs`
   - Validates `source_type` and non-empty `pipeline`.
   - `source_type=s3_presigned`:
     - Creates job with `created` status and object key `uploads/{job_id}/source`.
     - Returns real `presigned_put_url`.
   - `source_type=local_file`:
     - Requires request `object_key` as local filesystem source path.
2. `POST /v1/jobs/{id}/start`
   - Looks up job by ID.
   - Verifies source object exists before enqueue:
     - local file existence check for `local_file`.
     - object existence check for `s3_presigned`.
   - Enqueues `image:process` task.
   - Marks job as `queued`.
3. Worker lifecycle updates persisted job status to `processing`, then `succeeded` or `failed`.

Current task:

1. Type: `image:process`
2. Payload: `job_id`, `source_type`, `webhook_url`, `object_key`, `pipeline`, `requested_at`.

Current source behavior:

1. `source_type=local_file`: `object_key` is treated as a local filesystem path by worker pipeline.
2. `source_type=s3_presigned`: worker fetches source from object storage and emits outputs to `outputs/{job_id}/...`.

Do not change existing field names casually. If contract changes are needed, update API handlers, task parser, tests, and README examples together.

## 10. Testing Expectations

Before considering work complete:

1. Run `go test ./...`.
2. If you changed formatting-sensitive files, run `gofmt`.
3. If dependencies changed, run `go mod tidy`.
4. If infra behavior changed, provide local verification steps (`curl`, logs, expected output).

If any check cannot run in current environment, explicitly state what was skipped and why.

## 11. Documentation Synchronization Rules

Any meaningful code or behavior change should trigger doc review:

1. `README.md` for user-facing run/usage changes.
2. `docs/ROADMAP.md` for phase/status changes.
3. `AGENTS.md` for agent workflow, architecture truth, commands, constraints, and next steps.

Avoid stale docs. Treat docs as part of the implementation.

## 12. Mandatory End-Of-Task Checklist (Required)

At the end of every task, the agent must do all of the following:

1. Re-open and review `AGENTS.md`.
2. Update `AGENTS.md` if any command, architecture detail, policy, file map, workflow, or phase status changed.
3. Re-open and review `Project Next Steps` in `AGENTS.md`.
4. Update next-step statuses and priorities to reflect current repo reality.
5. In the final user response, state that `AGENTS.md` was reviewed and whether it was updated.

If no update is needed, explicitly state: `AGENTS.md reviewed; no changes required`.

## 13. Project Next Steps (Keep This Section Up To Date)

Status key: `pending`, `in_progress`, `blocked`, `done`

1. `done` Phase 2: implement `govips` pipeline package in worker.
2. `done` Phase 2: add local integration test for file-in -> transform -> file-out.
3. `done` Phase 2: replace `build/Dockerfile.worker-vips` scaffold with real multi-stage libvips/libheif build.
4. `done` Phase 3: implement MinIO/S3 client package and real presigned PUT URL generation in `POST /v1/jobs`.
5. `done` Phase 3: add upload existence checks before enqueue in `/v1/jobs/{id}/start`.
6. `done` Phase 3: add webhook delivery with retry/backoff and signed payload.
7. `done` Phase 3: replace in-memory job store with Postgres-backed persistence.
8. `pending` Phase 4: add Redis token-bucket rate limiting middleware in API.
9. `pending` Phase 4: add `usage_logs` schema and writes (`pixels_processed`, `bytes_saved`, `compute_time_ms`).
10. `pending` Phase 4: add OpenTelemetry and Prometheus metrics for queue and processing.
11. `pending` Phase 4: publish repeatable benchmark method/results in `README.md`.

## 14. Update Protocol For This File

When editing `AGENTS.md`:

1. Keep instructions concrete, testable, and repo-specific.
2. Remove outdated statements immediately; do not leave “TODO maybe” ambiguity.
3. Update `Last updated` date.
4. Keep `Project Next Steps` synchronized with `docs/ROADMAP.md`.
5. Prefer short command lists and explicit file paths over broad prose.

This file is a living contract for agent quality and maintenance velocity.
