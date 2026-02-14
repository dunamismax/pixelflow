# PixelFlow

PixelFlow is a high-throughput, asynchronous image processing pipeline for teams that need durable job orchestration, object-storage workflows, and production-grade observability.

## Hero

PixelFlow separates control-plane API operations from data-plane image processing so you can queue, process, and track image jobs without pushing heavy image work through your HTTP layer.

- Control plane API for job creation and enqueueing
- Asynq-based worker for resize and watermark transforms
- Local file and MinIO/S3 presigned source flows
- Postgres-backed job state and usage metering
- Prometheus metrics and OpenTelemetry tracing

## Trust Signals

![Go Version](https://img.shields.io/badge/go-1.24%2B-00ADD8?logo=go&logoColor=white)
![Docker Compose](https://img.shields.io/badge/docker-compose_v2-2496ED?logo=docker&logoColor=white)
![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)

- Release status: core pipeline phases are complete, with Phase 5 hardening tracked in `docs/ROADMAP.md`.
- Last command validation in this repo: `2026-02-14`.

## Quick Start

### Prerequisites

- Go `1.24+`
- Docker Engine with Docker Compose v2
- `curl` for API verification
- Git

### Run

1. Clone and enter the repository:

```bash
git clone https://github.com/dunamismax/pixelflow.git
cd pixelflow
```

1. Start local infrastructure:

```bash
docker compose up -d
```

1. Start the API (terminal 1):

```bash
go run ./cmd/api
```

1. Start the worker (terminal 2):

```bash
go run ./cmd/worker
```

1. Verify API health:

```bash
curl http://localhost:8080/healthz
```

Expected result:

- Health response: `{"status":"ok"}`
- API metrics: `http://localhost:9090/metrics`
- Worker metrics: `http://localhost:9091/metrics`

## Features

- `Job API`: create and start jobs via `POST /v1/jobs` and `POST /v1/jobs/{id}/start`.
- `Dual source modes`: process `local_file` sources or `s3_presigned` object-storage uploads.
- `Pipeline actions`: resize and text watermark transforms with explicit step definitions.
- `Durable state`: persisted job lifecycle in Postgres (`created`, `queued`, `processing`, `succeeded`, `failed`).
- `Usage metering`: worker writes `usage_logs` with pixels processed, bytes saved, and compute time.
- `Rate limiting`: Redis token bucket on mutating job endpoints.
- `Webhooks`: signed callback delivery with retry and exponential backoff.
- `Observability`: Prometheus metrics and OpenTelemetry traces in both API and worker.

## Tech Stack

| Layer | Technology | Purpose |
| --- | --- | --- |
| Language/runtime | [Go 1.24+](https://go.dev/) | API and worker services |
| Queue | [Asynq](https://github.com/hibiken/asynq) + [Redis](https://redis.io/) | Background task dispatch and processing |
| Database | [PostgreSQL](https://www.postgresql.org/) | Durable job and usage persistence |
| Object storage | [MinIO](https://min.io/) / S3-compatible APIs | Presigned upload flow and object IO |
| Image processing | Go stdlib + optional [`govips`](https://github.com/davidbyttow/govips) | Resize and watermark transforms |
| Metrics | [Prometheus client_golang](https://github.com/prometheus/client_golang) | API/worker metrics endpoints |
| Tracing | [OpenTelemetry](https://opentelemetry.io/) | Distributed tracing instrumentation |
| Containerization | Docker + Docker Compose | Local infra and deployable images |

## Project Structure

```text
pixelflow/
├── cmd/
│   ├── api/                     # API process bootstrap
│   └── worker/                  # Worker process bootstrap
├── internal/
│   ├── api/                     # HTTP handlers, tracing, metrics, rate limiting
│   ├── config/                  # Environment-driven config loader
│   ├── domain/                  # Job and usage domain models
│   ├── pipeline/                # Fetch/transform/emit image pipeline
│   ├── queue/                   # Asynq task contracts and enqueue client
│   ├── ratelimit/               # Redis token bucket implementation
│   ├── storage/                 # MinIO/S3 client wrapper
│   ├── store/                   # Postgres and memory stores
│   ├── telemetry/               # OpenTelemetry setup
│   ├── webhook/                 # Signed webhook client with retries
│   └── worker/                  # Worker server and task handlers
├── build/
│   ├── Dockerfile.api           # API container image
│   ├── Dockerfile.worker        # Worker container image (stdlib path)
│   └── Dockerfile.worker-vips   # Worker container image with govips/libvips
├── docs/
│   └── ROADMAP.md               # Phase roadmap and planned hardening work
├── docker-compose.yml           # Redis + Postgres + MinIO local stack
├── .env.example                 # Environment variable reference
└── Makefile                     # Common local developer targets
```

## Development Workflow and Common Commands

### Setup

```bash
docker compose up -d
```

### Run

```bash
go run ./cmd/api
go run ./cmd/worker
```

### Test

```bash
go test ./...
go test ./internal/pipeline -run TestLocalProcessor_FileInTransformFileOut -count=1
go test ./internal/pipeline -run ^$ -bench BenchmarkProcessor -benchmem -count=1
```

### Build

```bash
docker build -f build/Dockerfile.api -t pixelflow-api-test .
docker build -f build/Dockerfile.worker -t pixelflow-worker-test .
docker build -f build/Dockerfile.worker-vips -t pixelflow-worker-vips-test .
```

### Cleanup

```bash
docker compose down
```

## Deployment and Operations

### Deployment method

- Build deployable containers from `build/Dockerfile.api`, `build/Dockerfile.worker`, or `build/Dockerfile.worker-vips`.
- Run API and worker as separate services; connect both to shared Redis, Postgres, and S3-compatible storage.

### Environment and secrets

- Use `.env.example` as the full environment contract.
- Do not commit real credentials; provide runtime secrets via environment variables or secret managers.

### Health, logs, and monitoring entry points

- API health check: `GET /healthz`
- API metrics: `PIXELFLOW_API_METRICS_ADDR` (default `:9090`)
- Worker metrics: `WORKER_METRICS_ADDR` (default `:9091`)
- Infra logs: `docker compose logs --no-color --tail=50 redis postgres minio minio-init`

### Rollback notes

- Roll back by redeploying previously published API/worker images.
- Job/usage schemas are managed idempotently on startup by the Postgres store bootstrap.

## Security and Reliability Notes

- `Input validation`: API uses strict JSON decoding and rejects unknown fields.
- `Rate control`: Redis token bucket protects job mutation endpoints.
- `Webhook integrity`: callbacks are HMAC-SHA256 signed (`X-Pixelflow-Signature`) with timestamp and event headers.
- `Source verification`: `/v1/jobs/{id}/start` checks source existence before enqueueing.
- `Worker stability`: semaphore limits active heavy jobs (`WORKER_MAX_ACTIVE_JOBS`).
- `Durability`: job state and usage logs persist in Postgres.
- `Current identity model`: user identity is header-derived (`X-User-ID` by default); stronger authenticated propagation is tracked in Phase 5.

## Documentation

| Path | Purpose |
| --- | --- |
| [`docs/ROADMAP.md`](docs/ROADMAP.md) | Phase roadmap and pending hardening work |
| [`.env.example`](.env.example) | Supported environment variables and defaults |
| [`AGENTS.md`](AGENTS.md) | Repo-operating and maintenance contract |

## Contributing

Contributions are welcome through pull requests and issues.

- Open an issue for bugs, regressions, or feature proposals.
- Keep changes scoped and include tests for behavior changes.
- Run `go test ./...` before opening a PR.

## License

Licensed under the [MIT License](LICENSE).
