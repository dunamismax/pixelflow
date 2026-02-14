.PHONY: up down logs run-api run-worker tidy test

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f redis postgres minio

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

tidy:
	go mod tidy

test:
	go test ./...

