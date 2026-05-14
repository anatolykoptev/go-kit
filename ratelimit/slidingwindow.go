package ratelimit

// SlidingWindow is a Redis-backed sliding-window counter rate limiter.
//
// Algorithm (ported from oxpulse-admin internal/admin/rate_limit_redis.go):
// Each window is divided into 1-minute buckets. On Allow, the current bucket
// is incremented (INCR + EXPIRE) and all buckets in the window are summed via
// GET. If the sum exceeds Limit the call is denied.
//
// Key schema: <KeyPrefix>:<key>:<unix-minute-epoch>
// Reset:      SCAN + DEL matching <KeyPrefix>:<key>:*
//
// Bucket TTL is set to Window + 1 minute so tail buckets do not expire before
// the window slides past them.
//
// On Redis errors the limiter fails open (allowed=true, err non-nil) when
// FailOpen is true.

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// SlidingWindowConfig holds the configuration for a SlidingWindow limiter.
type SlidingWindowConfig struct {
	// Redis is the Redis client used to store counters. Must implement redis.Cmdable.
	Redis redis.Cmdable
	// KeyPrefix is prepended to all Redis keys: "<KeyPrefix>:<key>:<bucket-epoch>".
	KeyPrefix string
	// Window is the total sliding window duration. Must be a multiple of one minute.
	Window time.Duration
	// Limit is the maximum number of calls allowed in Window.
	Limit int
	// FailOpen controls behaviour on Redis errors: if true, the call is allowed.
	FailOpen bool
}

// SlidingWindow is a Redis-backed sliding-window rate limiter.
type SlidingWindow struct {
	cfg        SlidingWindowConfig
	bucketTTL  time.Duration
	numBuckets int
	now        func() time.Time // override in tests via wrapping; exposed for testing via config extension
}

// NewSlidingWindow creates a new SlidingWindow limiter from cfg.
//
// Window should be a multiple of one minute; sub-minute windows use
// second-granularity buckets when Window < 1 minute.
func NewSlidingWindow(cfg SlidingWindowConfig) *SlidingWindow {
	bucketSec := int64(60) // 1-minute bucket matches donor
	if cfg.Window < time.Minute {
		// For sub-minute windows, use 1-second buckets.
		bucketSec = 1
	}
	bucketDur := time.Duration(bucketSec) * time.Second
	numBuckets := int(cfg.Window / bucketDur)
	if numBuckets < 1 {
		numBuckets = 1
	}
	return &SlidingWindow{
		cfg:        cfg,
		bucketTTL:  cfg.Window + bucketDur, // slightly longer than window (donor pattern)
		numBuckets: numBuckets,
		now:        time.Now,
	}
}

// Allow records one attempt and reports whether the caller is within the rate
// limit. It returns (allowed, remaining, error).
//
// remaining is the number of calls still permitted in the current window after
// this call. On denial remaining is 0.
//
// On Redis errors the limiter fails open when FailOpen is true.
func (s *SlidingWindow) Allow(ctx context.Context, key string) (allowed bool, remaining int, err error) {
	now := s.now()
	bucketSec := s.bucketTTLSeconds()
	currentBucket := now.Unix() / bucketSec

	pipe := s.cfg.Redis.Pipeline()
	bucketKey := s.bucketKey(key, currentBucket)
	incrCmd := pipe.Incr(ctx, bucketKey)
	pipe.Expire(ctx, bucketKey, s.bucketTTL)

	getCmds := make([]*redis.StringCmd, s.numBuckets)
	for i := 0; i < s.numBuckets; i++ {
		b := currentBucket - int64(i)
		getCmds[i] = pipe.Get(ctx, s.bucketKey(key, b))
	}

	if _, execErr := pipe.Exec(ctx); execErr != nil && execErr != redis.Nil {
		if s.cfg.FailOpen {
			return true, 0, execErr
		}
		return false, 0, execErr
	}
	_ = incrCmd // result included in GET sum below (same key)

	var total int64
	for _, cmd := range getCmds {
		if v, cmdErr := cmd.Int64(); cmdErr == nil {
			total += v
		}
	}

	if total > int64(s.cfg.Limit) {
		return false, 0, nil
	}
	rem := s.cfg.Limit - int(total)
	return true, rem, nil
}

// Reset deletes all bucket keys for key using SCAN + DEL.
// Called on successful login so isolated typos do not erode the budget.
//
// Note: the donor used SCAN+DEL (not KEYS+DEL) to avoid blocking Redis.
// This implementation preserves that choice.
func (s *SlidingWindow) Reset(ctx context.Context, key string) error {
	pattern := fmt.Sprintf("%s:%s:*", s.cfg.KeyPrefix, key)
	var cursor uint64
	for {
		keys, nextCursor, err := s.cfg.Redis.Scan(ctx, cursor, pattern, 50).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := s.cfg.Redis.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			return nil
		}
	}
}

// bucketKey returns the Redis key for a given key and bucket epoch.
func (s *SlidingWindow) bucketKey(key string, bucketEpoch int64) string {
	return fmt.Sprintf("%s:%s:%d", s.cfg.KeyPrefix, key, bucketEpoch)
}

// bucketTTLSeconds returns the bucket duration in seconds.
func (s *SlidingWindow) bucketTTLSeconds() int64 {
	if s.cfg.Window < time.Minute {
		return 1
	}
	return 60
}
