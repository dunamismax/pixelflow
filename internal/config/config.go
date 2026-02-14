package config

import (
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
)

type Config struct {
	API       APIConfig
	Queue     QueueConfig
	Worker    WorkerConfig
	Storage   StorageConfig
	Database  DatabaseConfig
	Webhook   WebhookConfig
	Telemetry TelemetryConfig
}

type APIConfig struct {
	Addr              string
	MetricsAddr       string
	RateLimitEnabled  bool
	RateLimitCapacity int
	RateLimitWindow   time.Duration
	RateLimitUserID   string
}

type QueueConfig struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	Name          string
}

func (q QueueConfig) RedisClientOpt() asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     q.RedisAddr,
		Password: q.RedisPassword,
		DB:       q.RedisDB,
	}
}

type WorkerConfig struct {
	Concurrency    int
	MaxActiveJobs  int
	LocalOutputDir string
	MetricsAddr    string
}

type StorageConfig struct {
	Endpoint         string
	AccessKey        string
	SecretKey        string
	Bucket           string
	UseSSL           bool
	PresignPutExpiry time.Duration
}

type DatabaseConfig struct {
	DSN string
}

type WebhookConfig struct {
	SigningSecret  string
	Timeout        time.Duration
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

type TelemetryConfig struct {
	TracesExporter    string
	OTLPTraceEndpoint string
	OTLPInsecure      bool
}

func Load() Config {
	defaultWorkerSlots := max(1, runtime.NumCPU()/2)

	return Config{
		API: APIConfig{
			Addr:              env("PIXELFLOW_API_ADDR", ":8080"),
			MetricsAddr:       env("PIXELFLOW_API_METRICS_ADDR", ":9090"),
			RateLimitEnabled:  envBool("PIXELFLOW_API_RATE_LIMIT_ENABLED", true),
			RateLimitCapacity: envInt("PIXELFLOW_API_RATE_LIMIT_CAPACITY", 60),
			RateLimitWindow:   envDuration("PIXELFLOW_API_RATE_LIMIT_WINDOW", time.Minute),
			RateLimitUserID:   env("PIXELFLOW_API_RATE_LIMIT_USER_ID_HEADER", "X-User-ID"),
		},
		Queue: QueueConfig{
			RedisAddr:     env("REDIS_ADDR", "localhost:6379"),
			RedisPassword: env("REDIS_PASSWORD", ""),
			RedisDB:       envInt("REDIS_DB", 0),
			Name:          env("ASYNC_QUEUE", "default"),
		},
		Worker: WorkerConfig{
			Concurrency:    envInt("WORKER_CONCURRENCY", max(2, runtime.NumCPU())),
			MaxActiveJobs:  envInt("WORKER_MAX_ACTIVE_JOBS", defaultWorkerSlots),
			LocalOutputDir: env("WORKER_LOCAL_OUTPUT_DIR", "./.pixelflow-output"),
			MetricsAddr:    env("WORKER_METRICS_ADDR", ":9091"),
		},
		Storage: StorageConfig{
			Endpoint:         env("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey:        env("MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey:        env("MINIO_SECRET_KEY", "minioadmin"),
			Bucket:           env("MINIO_BUCKET", "pixelflow-jobs"),
			UseSSL:           envBool("MINIO_USE_SSL", false),
			PresignPutExpiry: envDuration("MINIO_PRESIGN_PUT_EXPIRY", 15*time.Minute),
		},
		Database: DatabaseConfig{
			DSN: env("POSTGRES_DSN", "postgres://pixelflow:pixelflow@localhost:5432/pixelflow?sslmode=disable"),
		},
		Webhook: WebhookConfig{
			SigningSecret:  env("WEBHOOK_SIGNING_SECRET", "pixelflow-dev-signing-secret"),
			Timeout:        envDuration("WEBHOOK_TIMEOUT", 10*time.Second),
			MaxAttempts:    envInt("WEBHOOK_MAX_ATTEMPTS", 5),
			InitialBackoff: envDuration("WEBHOOK_INITIAL_BACKOFF", 1*time.Second),
			MaxBackoff:     envDuration("WEBHOOK_MAX_BACKOFF", 30*time.Second),
		},
		Telemetry: TelemetryConfig{
			TracesExporter:    env("OTEL_TRACES_EXPORTER", "none"),
			OTLPTraceEndpoint: env("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
			OTLPInsecure:      envBool("OTEL_EXPORTER_OTLP_INSECURE", true),
		},
	}
}

func env(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := env(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := env(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := env(key, "")
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
