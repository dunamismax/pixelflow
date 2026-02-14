# PixelFlow

High-Throughput, Asynchronous Image Processing Pipeline

## Phase Status

- `Phase 1`: complete (walking skeleton)
- `Phase 2`: scaffolded (pipeline interfaces and worker throttle in place)
- `Phase 3`: scaffolded (MinIO/S3 config + presigned URL integration path)
- `Phase 4`: partially scaffolded (concurrency limiter implemented)

## Architecture

- Control Plane (API): validates requests, creates jobs, and enqueues processing.
- Data Plane (Worker): consumes queued jobs and runs heavy image work.
- Infra: Redis (queue), Postgres (future persistent job/user usage), MinIO (object storage).

## Repo Layout

```text
cmd/
  api/       # HTTP control-plane service
  worker/    # asynchronous data-plane worker
internal/
  api/       # HTTP handlers and route wiring
  config/    # environment-driven config
  domain/    # request/job models
  id/        # lightweight ID generation
  queue/     # asynq task contracts + enqueue client
  store/     # in-memory job store (Phase 1 placeholder for Postgres)
  worker/    # asynq worker server and handlers
pkg/
  version/   # shared version metadata
build/
  Dockerfile.api          # multi-stage API image
  Dockerfile.worker       # multi-stage worker image (Phase 1 compatible)
  Dockerfile.worker-vips  # scaffold for libvips/libheif runtime image
```

## Local Stack (Docker Compose)

Starts:

- Redis: `localhost:6379`
- Postgres: `localhost:5432` (`pixelflow/pixelflow`)
- MinIO API: `localhost:9000`
- MinIO Console: `localhost:9001`

```bash
docker compose up -d
```

## Run Services

```bash
go run ./cmd/api
```

```bash
go run ./cmd/worker
```

## Phase 1 API Flow

1. Create a job with pipeline instructions.
2. Start the job (this enqueues dummy processing payload to Redis/Asynq).
3. Worker receives the task and logs `Working...`.

### 1) Create Job

```bash
curl -X POST http://localhost:8080/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "source_type": "s3_presigned",
    "webhook_url": "https://client-site.com/hooks/pixel-done",
    "pipeline": [
      {
        "id": "thumb_small",
        "action": "resize",
        "width": 150,
        "format": "webp",
        "quality": 80
      }
    ]
  }'
```

### 2) Start Job

Use the returned `job_id`:

```bash
curl -X POST http://localhost:8080/v1/jobs/<job_id>/start
```

### 3) Health Check

```bash
curl http://localhost:8080/healthz
```

## Configuration

See `.env.example` for all supported environment variables.

## Next Phases

- Phase 2: wire `govips` pipeline in worker handler.
- Phase 3: generate real MinIO/S3 presigned URLs in `POST /v1/jobs`.
- Phase 4: add metrics, usage metering, and Redis-backed rate limiting middleware.

Detailed roadmap: `docs/ROADMAP.md`.
