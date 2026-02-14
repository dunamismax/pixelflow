# PixelFlow

High-Throughput, Asynchronous Image Processing Pipeline

## Phase Status

- `Phase 1`: complete (walking skeleton)
- `Phase 2`: implemented for local file processing (`resize`, `watermark`) with optional `govips` runtime
- `Phase 3`: complete (MinIO/S3 presigned upload flow, upload checks, webhook delivery, Postgres job store)
- `Phase 4`: partially scaffolded (concurrency limiter implemented)

## Architecture

- Control Plane (API): validates requests, persists jobs in Postgres, generates presigned upload URLs, and enqueues processing.
- Data Plane (Worker): consumes queued jobs, processes images from local file or object storage, emits outputs, and sends signed webhook callbacks.
- Infra: Redis (queue), Postgres (durable jobs), MinIO (object storage).

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
  pipeline/  # fetch/transform/emit image pipeline stages
  queue/     # asynq task contracts + enqueue client
  storage/   # MinIO/S3 client wrapper (presign/stat/get/put)
  store/     # job persistence implementations (memory + Postgres)
  webhook/   # webhook sender with signed payload + retry/backoff
  worker/    # asynq worker server and handlers
pkg/
  version/   # shared version metadata
build/
  Dockerfile.api          # multi-stage API image
  Dockerfile.worker       # multi-stage worker image (Phase 1 compatible)
  Dockerfile.worker-vips  # multi-stage CGO/libvips worker build (`-tags govips`)
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

Worker writes Phase 2 local pipeline outputs to `WORKER_LOCAL_OUTPUT_DIR` (default `./.pixelflow-output`).

## API Flow (Current)

1. Create a job with pipeline instructions.
2. For `source_type=s3_presigned`, upload source bytes to the returned presigned URL.
3. Start the job (API validates the source object exists before enqueueing).
4. Worker behavior:
   - `source_type=local_file`: fetches local source path and writes local outputs.
   - `source_type=s3_presigned`: fetches input from MinIO/S3 and writes outputs to `outputs/{job_id}/...`.
5. Job status transitions are persisted in Postgres (`created` -> `queued` -> `processing` -> `succeeded|failed`).
6. Worker sends signed webhook callbacks (`job.completed` / `job.failed`) with retry/backoff.

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

### 2) Upload Source (For `s3_presigned`)

Use the returned `upload.presigned_put_url`:

```bash
curl -X PUT "<presigned_put_url>" \
  -H "Content-Type: image/png" \
  --data-binary "@./source.png"
```

### 3) Start Job

Use the returned `job_id`:

```bash
curl -X POST http://localhost:8080/v1/jobs/<job_id>/start
```

### 4) Health Check

```bash
curl http://localhost:8080/healthz
```

## Configuration

See `.env.example` for all supported environment variables.

## Phase 2 Local Pipeline Check

Run the local file integration test:

```bash
go test ./internal/pipeline -run TestLocalProcessor_FileInTransformFileOut
```

Build the `govips` worker container:

```bash
docker build -f build/Dockerfile.worker-vips -t pixelflow-worker-vips .
```

## Next Phases

- Phase 4: add metrics, usage metering, and Redis-backed rate limiting middleware.

Detailed roadmap: `docs/ROADMAP.md`.
