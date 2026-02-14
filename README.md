# PixelFlow

High-Throughput, Asynchronous Image Processing Pipeline

## Phase Status

- `Phase 1`: complete (walking skeleton)
- `Phase 2`: implemented for local file processing (`resize`, `watermark`) with optional `govips` runtime
- `Phase 3`: complete (MinIO/S3 presigned upload flow, upload checks, webhook delivery, Postgres job store)
- `Phase 4`: complete (Redis rate limiting, usage metering, Prometheus metrics, OpenTelemetry tracing, benchmarks)

## Architecture

- Control Plane (API): validates requests, persists jobs in Postgres, generates presigned upload URLs, and enqueues processing.
- Data Plane (Worker): consumes queued jobs, processes images from local file or object storage, emits outputs, and sends signed webhook callbacks.
- Infra: Redis (queue), Postgres (durable jobs), MinIO (object storage).
- Production controls: Redis token-bucket rate limiting in API (`POST /v1/jobs`, `POST /v1/jobs/{id}/start`).
- Observability: Prometheus metrics endpoints in both API and worker processes; OpenTelemetry tracing for API requests and worker job processing.

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

Metrics endpoints:

- API metrics: `http://localhost:9090/metrics` (`PIXELFLOW_API_METRICS_ADDR`)
- Worker metrics: `http://localhost:9091/metrics` (`WORKER_METRICS_ADDR`)

## API Flow (Current)

1. Create a job with pipeline instructions.
2. For `source_type=s3_presigned`, upload source bytes to the returned presigned URL.
3. Start the job (API validates the source object exists before enqueueing).
4. Worker behavior:
   - `source_type=local_file`: fetches local source path and writes local outputs.
   - `source_type=s3_presigned`: fetches input from MinIO/S3 and writes outputs to `outputs/{job_id}/...`.
5. Job status transitions are persisted in Postgres (`created` -> `queued` -> `processing` -> `succeeded|failed`).
6. Worker sends signed webhook callbacks (`job.completed` / `job.failed`) with retry/backoff.
7. API rate limiting is applied per user header (`X-User-ID` by default) and route using a Redis token bucket.
8. Worker writes `usage_logs` in Postgres for successful jobs (`pixels_processed`, `bytes_saved`, `compute_time_ms`).

### 1) Create Job

```bash
curl -X POST http://localhost:8080/v1/jobs \
  -H "Content-Type: application/json" \
  -H "X-User-ID: demo-user" \
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

Key Phase 4 env vars:

- `PIXELFLOW_API_RATE_LIMIT_ENABLED` (default `true`)
- `PIXELFLOW_API_RATE_LIMIT_CAPACITY` (default `60`)
- `PIXELFLOW_API_RATE_LIMIT_WINDOW` (default `1m`)
- `PIXELFLOW_API_RATE_LIMIT_USER_ID_HEADER` (default `X-User-ID`)
- `PIXELFLOW_API_METRICS_ADDR` (default `:9090`)
- `WORKER_METRICS_ADDR` (default `:9091`)
- `OTEL_TRACES_EXPORTER` (`none`, `stdout`, or `otlp`; default `none`)
- `OTEL_EXPORTER_OTLP_ENDPOINT` (required when `OTEL_TRACES_EXPORTER=otlp`)
- `OTEL_EXPORTER_OTLP_INSECURE` (default `true`)

## Phase 2 Local Pipeline Check

Run the local file integration test:

```bash
go test ./internal/pipeline -run TestLocalProcessor_FileInTransformFileOut
```

Build the `govips` worker container:

```bash
docker build -f build/Dockerfile.worker-vips -t pixelflow-worker-vips .
```

## Benchmark Method And Results

Repeatable benchmark command:

```bash
go test ./internal/pipeline -run ^$ -bench BenchmarkProcessor -benchmem -count=3
```

Benchmark environment (`2026-02-14`):

- `goos=windows`
- `goarch=amd64`
- `cpu=Intel(R) Core(TM) Ultra 9 275HX`
- Build tags: default stdlib transformer path (no `govips`)

Latest results:

| Benchmark | Mean ns/op | Mean ms/op | Mean allocs/op | Mean bytes/op |
| --- | ---: | ---: | ---: | ---: |
| `BenchmarkProcessorResize` | `12,541,844` | `12.54` | `230,435` | `10,244,979` |
| `BenchmarkProcessorWatermark` | `26,361,288` | `26.36` | `58` | `17,557,174` |

Detailed roadmap: `docs/ROADMAP.md`.
