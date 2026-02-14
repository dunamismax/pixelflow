package config

import (
	"os"
	"runtime"
	"strconv"

	"github.com/hibiken/asynq"
)

type Config struct {
	API      APIConfig
	Queue    QueueConfig
	Worker   WorkerConfig
	Storage  StorageConfig
	Database DatabaseConfig
}

type APIConfig struct {
	Addr string
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
}

type StorageConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type DatabaseConfig struct {
	DSN string
}

func Load() Config {
	defaultWorkerSlots := max(1, runtime.NumCPU()/2)

	return Config{
		API: APIConfig{
			Addr: env("PIXELFLOW_API_ADDR", ":8080"),
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
		},
		Storage: StorageConfig{
			Endpoint:  env("MINIO_ENDPOINT", "localhost:9000"),
			AccessKey: env("MINIO_ACCESS_KEY", "minioadmin"),
			SecretKey: env("MINIO_SECRET_KEY", "minioadmin"),
			Bucket:    env("MINIO_BUCKET", "pixelflow-jobs"),
			UseSSL:    envBool("MINIO_USE_SSL", false),
		},
		Database: DatabaseConfig{
			DSN: env("POSTGRES_DSN", "postgres://pixelflow:pixelflow@localhost:5432/pixelflow?sslmode=disable"),
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
