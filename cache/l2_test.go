package cache_test

import (
	"context"
	"errors"
	"testing"

	"github.com/anatolykoptev/go-kit/cache"
)

func TestRedisL2_NilReceiver_Get(t *testing.T) {
	var r *cache.RedisL2
	_, err := r.Get(context.Background(), "key")
	if !errors.Is(err, cache.ErrCacheMiss) {
		t.Errorf("nil receiver Get: got %v, want ErrCacheMiss", err)
	}
}

func TestRedisL2_NilReceiver_Set(t *testing.T) {
	var r *cache.RedisL2
	err := r.Set(context.Background(), "key", []byte("v"), 0)
	if !errors.Is(err, cache.ErrL2Unavailable) {
		t.Errorf("nil receiver Set: got %v, want ErrL2Unavailable", err)
	}
}

func TestRedisL2_NilReceiver_Del(t *testing.T) {
	var r *cache.RedisL2
	err := r.Del(context.Background(), "key")
	if !errors.Is(err, cache.ErrL2Unavailable) {
		t.Errorf("nil receiver Del: got %v, want ErrL2Unavailable", err)
	}
}

func TestRedisL2_NilReceiver_Close(t *testing.T) {
	var r *cache.RedisL2
	if err := r.Close(); err != nil {
		t.Errorf("nil receiver Close: got %v, want nil", err)
	}
}

func TestErrCacheMiss_Sentinel(t *testing.T) {
	if !errors.Is(cache.ErrCacheMiss, cache.ErrCacheMiss) {
		t.Error("ErrCacheMiss should match itself")
	}
	if errors.Is(cache.ErrCacheMiss, cache.ErrL2Unavailable) {
		t.Error("ErrCacheMiss should not match ErrL2Unavailable")
	}
}
