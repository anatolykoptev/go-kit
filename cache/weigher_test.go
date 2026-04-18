package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
)

func TestWeigher_EvictsWhenWeightExceedsMax(t *testing.T) {
	c := cache.New(cache.Config{
		L1MaxItems: 1000, // high enough that entry count is NOT the bound
		L1TTL:      time.Hour,
		MaxWeight:  100, // bytes
		Weigher: func(_ string, v []byte) int64 {
			return int64(len(v))
		},
	})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "k1", []byte("aaaaaaaa"))   // 8 bytes
	c.Set(ctx, "k2", []byte("bbbbbbbbbb")) // 10 bytes
	c.Set(ctx, "k3", []byte("large payload taking 90+ bytes, designed to push total weight well past one hundred"))

	// Total weight now > 100; at least one earlier key must have been evicted.
	v1, ok1 := c.Get(ctx, "k1")
	v2, ok2 := c.Get(ctx, "k2")
	v3, ok3 := c.Get(ctx, "k3")
	_ = v1
	_ = v2
	_ = v3
	evicted := 0
	for _, ok := range []bool{ok1, ok2, ok3} {
		if !ok {
			evicted++
		}
	}
	if evicted == 0 {
		t.Fatal("expected at least one eviction after exceeding MaxWeight")
	}
}

func TestWeigher_NilMeansNoWeightTracking(t *testing.T) {
	// With Weigher=nil and MaxWeight=0, behavior must be unchanged (S3-FIFO on count).
	c := cache.New(cache.Config{L1MaxItems: 2, L1TTL: time.Hour})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "k1", []byte("v1"))
	c.Set(ctx, "k2", []byte("v2"))
	c.Set(ctx, "k3", []byte("v3"))
	// Classic count-based eviction — exactly one evicted.
	stats := c.Stats()
	if stats.Evictions == 0 {
		t.Fatal("expected count-based eviction when Weigher is nil")
	}
}

func TestWeigher_StatsIncludeTotalWeight(t *testing.T) {
	c := cache.New(cache.Config{
		L1MaxItems: 100,
		L1TTL:      time.Hour,
		MaxWeight:  10000,
		Weigher:    func(_ string, v []byte) int64 { return int64(len(v)) },
	})
	defer c.Close()

	ctx := context.Background()
	c.Set(ctx, "a", []byte("1234567890")) // 10
	c.Set(ctx, "b", []byte("abcd"))       // 4
	stats := c.Stats()
	if stats.TotalWeight != 14 {
		t.Fatalf("TotalWeight = %d, want 14", stats.TotalWeight)
	}
}
