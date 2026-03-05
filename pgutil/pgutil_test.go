package pgutil

import (
	"context"
	"errors"
	"testing"
)

type fakePool struct{ id int }

func TestConnect_Success(t *testing.T) {
	pool, err := Connect(context.Background(), Options{MaxAttempts: 3}, func(_ context.Context) (*fakePool, error) {
		return &fakePool{id: 42}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool == nil || pool.id != 42 {
		t.Fatalf("expected pool with id=42, got %v", pool)
	}
}

func TestConnect_RetryThenSuccess(t *testing.T) {
	attempts := 0
	pool, err := Connect(context.Background(), Options{MaxAttempts: 5}, func(_ context.Context) (*fakePool, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("connection refused")
		}
		return &fakePool{id: 1}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestConnect_AllFailRequired(t *testing.T) {
	_, err := Connect(context.Background(), Options{MaxAttempts: 2}, func(_ context.Context) (*fakePool, error) {
		return nil, errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("expected error for required connection")
	}
}

func TestConnect_AllFailOptional(t *testing.T) {
	pool, err := Connect(context.Background(), Options{MaxAttempts: 2, Optional: true}, func(_ context.Context) (*fakePool, error) {
		return nil, errors.New("connection refused")
	})
	if err != nil {
		t.Fatalf("expected nil error for optional, got: %v", err)
	}
	if pool != nil {
		t.Fatal("expected nil pool for optional degraded mode")
	}
}

func TestConnect_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Connect(ctx, Options{MaxAttempts: 10}, func(_ context.Context) (*fakePool, error) {
		return nil, errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}
