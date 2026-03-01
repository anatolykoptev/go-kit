package cache

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// L2 is an optional second-tier cache (typically Redis).
// Get returns the value and nil error on hit, a non-nil error on miss or failure.
// Implementations must be safe for concurrent use.
type L2 interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, data []byte, ttl time.Duration) error
	Del(ctx context.Context, key string) error
	Close() error
}

// RedisL2 implements L2 using Redis.
type RedisL2 struct {
	rdb    *redis.Client
	prefix string
}

// NewRedisL2 connects to Redis and returns an L2 store.
// Returns nil if the URL is empty or Redis is unreachable (logs a warning).
func NewRedisL2(redisURL string, db int, prefix string) *RedisL2 {
	if redisURL == "" {
		return nil
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Warn("cache: invalid redis URL, L2 disabled", slog.Any("error", err))
		return nil
	}
	opts.DB = db

	rdb := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("cache: redis unreachable, L2 disabled", slog.Any("error", err))
		rdb.Close()
		return nil
	}

	return &RedisL2{rdb: rdb, prefix: prefix}
}

func (r *RedisL2) key(k string) string {
	if r.prefix == "" {
		return k
	}
	return r.prefix + k
}

// Get retrieves a value from Redis by key.
func (r *RedisL2) Get(ctx context.Context, key string) ([]byte, error) {
	return r.rdb.Get(ctx, r.key(key)).Bytes()
}

// Set stores a value in Redis with the given TTL.
func (r *RedisL2) Set(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	return r.rdb.Set(ctx, r.key(key), data, ttl).Err()
}

// Del removes a key from Redis.
func (r *RedisL2) Del(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, r.key(key)).Err()
}

// Close closes the underlying Redis client.
func (r *RedisL2) Close() error {
	return r.rdb.Close()
}
