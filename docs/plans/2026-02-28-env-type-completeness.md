# env Phase B: Type Completeness Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Uint, Uint64, Map, URL types (with *E and Must* variants) — completing the env package's type coverage to match competitors.

**Architecture:** Same patterns as Phase A. Base function (silent default), *E variant (returns ParseError), Must* variant (panics). All in existing files. Duration format already done in Phase A.

**Tech Stack:** Go stdlib only (`net/url` added for URL)

---

### Task 1: Uint and Uint64 (+ UintE, Uint64E, MustUint, MustUint64)

**Files:** env/env.go, env/must.go, env/env_test.go, env/must_test.go

Add after Float/FloatE block in env.go:

```go
// Uint returns the environment variable as a uint, or def if not set or invalid.
func Uint(key string, def uint) uint {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseUint(v, 10, strconv.IntSize); err == nil {
			return uint(n)
		}
	}
	return def
}

// UintE is like Uint but returns a ParseError if the variable is set but not a valid uint.
func UintE(key string, def uint) (uint, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.ParseUint(v, 10, strconv.IntSize)
	if err != nil {
		return def, &ParseError{Key: key, Value: v, Type: "uint", Err: err}
	}
	return uint(n), nil
}

// Uint64 returns the environment variable as uint64, or def if not set or invalid.
func Uint64(key string, def uint64) uint64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

// Uint64E is like Uint64 but returns a ParseError if the variable is set but not a valid uint64.
func Uint64E(key string, def uint64) (uint64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return def, &ParseError{Key: key, Value: v, Type: "uint64", Err: err}
	}
	return n, nil
}
```

Add to must.go:
```go
// MustUint is like Uint but panics if the variable is set to an invalid uint.
func MustUint(key string, def uint) uint {
	v, err := UintE(key, def)
	if err != nil { panic(err) }
	return v
}

// MustUint64 is like Uint64 but panics if the variable is set to an invalid uint64.
func MustUint64(key string, def uint64) uint64 {
	v, err := Uint64E(key, def)
	if err != nil { panic(err) }
	return v
}
```

Tests: valid, not-set (default), invalid (negative number, text), Must* panic cases.

---

### Task 2: Map

**Files:** env/env.go, env/env_test.go

Add after Int64List in env.go:

```go
// Map returns a comma-separated list of key:value pairs as a map.
// Format: "k1:v1,k2:v2". Entries without ":" are silently skipped.
// Whitespace is trimmed from keys and values. Returns nil if not set.
func Map(key, def string) map[string]string {
	v := Str(key, def)
	if v == "" {
		return nil
	}
	m := make(map[string]string)
	for _, pair := range strings.Split(v, ",") {
		k, val, ok := strings.Cut(strings.TrimSpace(pair), ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		m[k] = strings.TrimSpace(val)
	}
	if len(m) == 0 {
		return nil
	}
	return m
}
```

Tests: valid "k1:v1,k2:v2", whitespace handling, entries without ":" skipped, not-set returns nil, default value, empty result returns nil.

---

### Task 3: URL (+ URLE, MustURL)

**Files:** env/env.go, env/must.go, env/env_test.go, env/must_test.go

Add `"net/url"` to imports in env.go. Add after Map:

```go
// URL returns the environment variable parsed as a URL, or the parsed def if not set or invalid.
// Returns nil only if both the variable and def are empty.
func URL(key string, def string) *url.URL {
	v := Str(key, def)
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil {
		if def != "" {
			u, _ = url.Parse(def)
			return u
		}
		return nil
	}
	return u
}

// URLE is like URL but returns a ParseError if the variable is set but not a valid URL.
func URLE(key string, def string) (*url.URL, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		if def == "" {
			return nil, nil
		}
		u, _ := url.Parse(def)
		return u, nil
	}
	u, err := url.Parse(v)
	if err != nil {
		return nil, &ParseError{Key: key, Value: v, Type: "url", Err: err}
	}
	return u, nil
}
```

Add MustURL to must.go:
```go
func MustURL(key string, def string) *url.URL {
	v, err := URLE(key, def)
	if err != nil { panic(err) }
	return v
}
```

Tests: valid URL, not-set returns parsed default, empty returns nil, MustURL panic on invalid.

---

### Task 4: Update README and ROADMAP

Update README env section with new types. Mark Phase B as DONE in ROADMAP.
