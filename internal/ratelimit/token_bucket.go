package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type Decision struct {
	Allowed    bool
	Remaining  int64
	RetryAfter time.Duration
}

type RedisTokenBucket struct {
	client      redis.UniversalClient
	capacity    int64
	refillPerMS float64
	ttl         time.Duration
	keyPrefix   string
	now         func() time.Time
	script      *redis.Script
}

func NewRedisTokenBucket(client redis.UniversalClient, capacity int, window time.Duration, keyPrefix string) (*RedisTokenBucket, error) {
	if client == nil {
		return nil, fmt.Errorf("redis client is required")
	}
	if capacity <= 0 {
		return nil, fmt.Errorf("capacity must be positive")
	}
	if window <= 0 {
		return nil, fmt.Errorf("window must be positive")
	}

	if strings.TrimSpace(keyPrefix) == "" {
		keyPrefix = "pixelflow:ratelimit"
	}

	windowMS := window.Milliseconds()
	if windowMS < 1 {
		windowMS = 1
	}

	return &RedisTokenBucket{
		client:      client,
		capacity:    int64(capacity),
		refillPerMS: float64(capacity) / float64(windowMS),
		ttl:         2 * window,
		keyPrefix:   keyPrefix,
		now:         time.Now,
		script: redis.NewScript(`
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_per_ms = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])
local ttl_ms = tonumber(ARGV[5])

local data = redis.call("HMGET", key, "tokens", "timestamp")
local tokens = tonumber(data[1])
local timestamp = tonumber(data[2])

if tokens == nil then
  tokens = capacity
end
if timestamp == nil then
  timestamp = now_ms
end

local elapsed = math.max(0, now_ms - timestamp)
tokens = math.min(capacity, tokens + (elapsed * refill_per_ms))

local allowed = 0
local retry_after_ms = 0
if tokens >= requested then
  tokens = tokens - requested
  allowed = 1
else
  retry_after_ms = math.ceil((requested - tokens) / refill_per_ms)
end

redis.call("HMSET", key, "tokens", tokens, "timestamp", now_ms)
redis.call("PEXPIRE", key, ttl_ms)

return {allowed, math.floor(tokens), retry_after_ms}
`),
	}, nil
}

func (l *RedisTokenBucket) Allow(ctx context.Context, subject string) (Decision, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		subject = "anonymous"
	}

	key := fmt.Sprintf("%s:%s", l.keyPrefix, subject)
	now := l.now().UTC().UnixMilli()
	raw, err := l.script.Run(
		ctx,
		l.client,
		[]string{key},
		l.capacity,
		l.refillPerMS,
		now,
		1,
		l.ttl.Milliseconds(),
	).Result()
	if err != nil {
		return Decision{}, fmt.Errorf("run token bucket script: %w", err)
	}

	values, ok := raw.([]any)
	if !ok || len(values) != 3 {
		return Decision{}, fmt.Errorf("invalid token bucket response")
	}

	allowed, err := toInt64(values[0])
	if err != nil {
		return Decision{}, fmt.Errorf("parse allow value: %w", err)
	}
	remaining, err := toInt64(values[1])
	if err != nil {
		return Decision{}, fmt.Errorf("parse remaining value: %w", err)
	}
	retryAfterMS, err := toInt64(values[2])
	if err != nil {
		return Decision{}, fmt.Errorf("parse retry-after value: %w", err)
	}

	return Decision{
		Allowed:    allowed == 1,
		Remaining:  remaining,
		RetryAfter: time.Duration(retryAfterMS) * time.Millisecond,
	}, nil
}

func toInt64(in any) (int64, error) {
	switch v := in.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported type %T", in)
	}
}
