package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/cache"
)

type testUser struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestSetJSON_GetJSON(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	want := testUser{Name: "Alice", Age: 30}
	if err := cache.SetJSON(c, ctx, "user:1", want); err != nil {
		t.Fatalf("SetJSON: %v", err)
	}

	got, ok, err := cache.GetJSON[testUser](c, ctx, "user:1")
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if !ok {
		t.Fatal("GetJSON: not ok")
	}
	if got != want {
		t.Errorf("GetJSON = %+v, want %+v", got, want)
	}
}

func TestGetJSON_Miss(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	_, ok, err := cache.GetJSON[testUser](c, context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("GetJSON should return false for missing key")
	}
}

func TestGetJSON_BadJSON(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	c.Set(ctx, "bad", []byte("not-json"))

	_, _, err := cache.GetJSON[testUser](c, ctx, "bad")
	if err == nil {
		t.Error("GetJSON should return error for bad JSON")
	}
}

func TestGetOrLoadJSON_Miss(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	want := testUser{Name: "Bob", Age: 25}
	got, err := cache.GetOrLoadJSON(c, ctx, "user:2", func(_ context.Context) (testUser, error) {
		return want, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoadJSON: %v", err)
	}
	if got != want {
		t.Errorf("GetOrLoadJSON = %+v, want %+v", got, want)
	}

	// Should be cached now.
	cached, ok, err := cache.GetJSON[testUser](c, ctx, "user:2")
	if err != nil {
		t.Fatalf("GetJSON after load: %v", err)
	}
	if !ok {
		t.Fatal("should be cached after GetOrLoadJSON")
	}
	if cached != want {
		t.Errorf("cached = %+v, want %+v", cached, want)
	}
}

func TestGetOrLoadJSON_Hit(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	pre := testUser{Name: "Cached", Age: 99}
	if err := cache.SetJSON(c, ctx, "pre", pre); err != nil {
		t.Fatalf("SetJSON: %v", err)
	}

	var called bool
	got, err := cache.GetOrLoadJSON(c, ctx, "pre", func(_ context.Context) (testUser, error) {
		called = true
		return testUser{}, nil
	})
	if err != nil {
		t.Fatalf("GetOrLoadJSON: %v", err)
	}
	if called {
		t.Error("loader should not be called on cache hit")
	}
	if got != pre {
		t.Errorf("got = %+v, want %+v", got, pre)
	}
}

func TestGetOrLoadJSON_LoaderError(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()

	errBoom := errors.New("boom")
	_, err := cache.GetOrLoadJSON(c, context.Background(), "fail",
		func(_ context.Context) (testUser, error) {
			return testUser{}, errBoom
		})
	if !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want %v", err, errBoom)
	}
}

func TestSetJSON_Slice(t *testing.T) {
	c := cache.New(cache.Config{L1MaxItems: 100, L1TTL: time.Minute})
	defer c.Close()
	ctx := context.Background()

	want := []string{"a", "b", "c"}
	if err := cache.SetJSON(c, ctx, "list", want); err != nil {
		t.Fatalf("SetJSON: %v", err)
	}

	got, ok, err := cache.GetJSON[[]string](c, ctx, "list")
	if err != nil || !ok {
		t.Fatalf("GetJSON: ok=%v, err=%v", ok, err)
	}
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("got = %v, want %v", got, want)
	}
}
