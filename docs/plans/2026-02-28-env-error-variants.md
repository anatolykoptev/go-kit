# env Phase A: Error Variants Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add error-returning variants, required validation, and lookup primitives to the `env` package — closing the biggest gap vs every competitor (caarlos0/env, kelseyhightower/envconfig, sethvargo/go-envconfig).

**Architecture:** New functions added to existing `env/env.go` — no new files, no new deps. Error types in a separate `env/errors.go` for clarity. All existing functions remain unchanged (100% backward compatible). New functions follow two patterns: `*E` (return error) and `Must*` (panic on failure).

**Tech Stack:** Go stdlib only (`os`, `strconv`, `strings`, `time`, `fmt`)

---

## API Overview

```go
// Error types
type NotSetError struct{ Key string }
type ParseError  struct{ Key, Value, Type string; Err error }

// Lookup primitives
func Lookup(key string) (string, bool)    // thin os.LookupEnv
func Exists(key string) bool              // is the variable set?

// Required (must be set and non-empty)
func Required(key string) (string, error) // returns NotSetError if missing/empty

// Error-returning variants (*E) — return ParseError on invalid value
// Not set → (def, nil) — that's not an error, just missing
func IntE(key string, def int) (int, error)
func Int64E(key string, def int64) (int64, error)
func FloatE(key string, def float64) (float64, error)
func BoolE(key string, def bool) (bool, error)
func DurationE(key string, def time.Duration) (time.Duration, error)

// Panic variants (Must*) — for fail-fast startup validation
func MustRequired(key string) string
func MustInt(key string, def int) int
func MustInt64(key string, def int64) int64
func MustFloat(key string, def float64) float64
func MustBool(key string, def bool) bool
func MustDuration(key string, def time.Duration) time.Duration
```

---

### Task 1: Error Types

**Files:**
- Create: `env/errors.go`
- Test: `env/errors_test.go`

**Step 1: Write tests for error types**

```go
// env/errors_test.go
package env_test

import (
	"errors"
	"testing"

	"github.com/anatolykoptev/go-kit/env"
)

func TestNotSetError_Message(t *testing.T) {
	err := &env.NotSetError{Key: "DB_URL"}
	want := `env: "DB_URL" is not set`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestParseError_Message(t *testing.T) {
	err := &env.ParseError{Key: "PORT", Value: "abc", Type: "int"}
	want := `env: cannot parse "PORT" value "abc" as int`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestParseError_Unwrap(t *testing.T) {
	inner := errors.New("strconv error")
	err := &env.ParseError{Key: "X", Value: "y", Type: "int", Err: inner}
	if !errors.Is(err, inner) {
		t.Error("ParseError should unwrap to inner error")
	}
}

func TestNotSetError_Is(t *testing.T) {
	err := &env.NotSetError{Key: "FOO"}
	var target *env.NotSetError
	if !errors.As(err, &target) {
		t.Error("errors.As should match *NotSetError")
	}
	if target.Key != "FOO" {
		t.Errorf("Key = %q, want %q", target.Key, "FOO")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestNotSetError|TestParseError' -v`
Expected: FAIL — types not defined

**Step 3: Implement error types**

```go
// env/errors.go
package env

import "fmt"

// NotSetError indicates a required environment variable is not set or empty.
type NotSetError struct {
	Key string
}

func (e *NotSetError) Error() string {
	return fmt.Sprintf("env: %q is not set", e.Key)
}

// ParseError indicates an environment variable was set but could not be
// parsed as the expected type.
type ParseError struct {
	Key   string
	Value string
	Type  string
	Err   error // underlying strconv/parse error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("env: cannot parse %q value %q as %s", e.Key, e.Value, e.Type)
}

func (e *ParseError) Unwrap() error {
	return e.Err
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestNotSetError|TestParseError' -v`
Expected: PASS (4 tests)

**Step 5: Run full test suite — no regressions**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -v`
Expected: All 16 existing + 4 new tests PASS

**Step 6: Commit**

```bash
cd /home/krolik/src/go-kit
git add env/errors.go env/errors_test.go
git commit -m "feat(env): add NotSetError and ParseError types"
```

---

### Task 2: Lookup and Exists

**Files:**
- Modify: `env/env.go` (add 2 functions at top, after existing `Str`)
- Test: `env/env_test.go` (append tests)

**Step 1: Write tests**

Append to `env/env_test.go`:

```go
func TestLookup_Set(t *testing.T) {
	t.Setenv("TEST_LOOKUP", "value")
	val, ok := env.Lookup("TEST_LOOKUP")
	if !ok || val != "value" {
		t.Errorf("Lookup = (%q, %v), want (%q, true)", val, ok, "value")
	}
}

func TestLookup_SetEmpty(t *testing.T) {
	t.Setenv("TEST_LOOKUP_EMPTY", "")
	val, ok := env.Lookup("TEST_LOOKUP_EMPTY")
	if !ok || val != "" {
		t.Errorf("Lookup = (%q, %v), want (%q, true)", val, ok, "")
	}
}

func TestLookup_NotSet(t *testing.T) {
	_, ok := env.Lookup("TEST_LOOKUP_MISSING_XYZ")
	if ok {
		t.Error("Lookup should return false for unset variable")
	}
}

func TestExists_True(t *testing.T) {
	t.Setenv("TEST_EXISTS", "x")
	if !env.Exists("TEST_EXISTS") {
		t.Error("Exists should return true for set variable")
	}
}

func TestExists_False(t *testing.T) {
	if env.Exists("TEST_EXISTS_MISSING_XYZ") {
		t.Error("Exists should return false for unset variable")
	}
}

func TestExists_Empty(t *testing.T) {
	t.Setenv("TEST_EXISTS_EMPTY", "")
	if !env.Exists("TEST_EXISTS_EMPTY") {
		t.Error("Exists should return true for set-but-empty variable")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestLookup|TestExists' -v`
Expected: FAIL — `env.Lookup` and `env.Exists` undefined

**Step 3: Implement**

Add to `env/env.go` after the import block, before `Str`:

```go
// Lookup returns the value of the environment variable and whether it was set.
// Unlike Str, it distinguishes between "not set" and "set to empty string".
func Lookup(key string) (string, bool) {
	return os.LookupEnv(key)
}

// Exists reports whether the environment variable is set (even if empty).
func Exists(key string) bool {
	_, ok := os.LookupEnv(key)
	return ok
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestLookup|TestExists' -v`
Expected: PASS (6 tests)

**Step 5: Full suite — no regressions**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -v`
Expected: All existing + 6 new tests PASS

**Step 6: Commit**

```bash
cd /home/krolik/src/go-kit
git add env/env.go env/env_test.go
git commit -m "feat(env): add Lookup and Exists primitives"
```

---

### Task 3: Required

**Files:**
- Modify: `env/env.go` (add `Required` function after `Exists`)
- Test: `env/env_test.go` (append tests)

**Step 1: Write tests**

Append to `env/env_test.go`:

```go
func TestRequired_Set(t *testing.T) {
	t.Setenv("TEST_REQ", "dbhost:5432")
	val, err := env.Required("TEST_REQ")
	if err != nil {
		t.Fatalf("Required returned error: %v", err)
	}
	if val != "dbhost:5432" {
		t.Errorf("Required = %q, want %q", val, "dbhost:5432")
	}
}

func TestRequired_NotSet(t *testing.T) {
	_, err := env.Required("TEST_REQ_MISSING_XYZ")
	if err == nil {
		t.Fatal("Required should return error for unset variable")
	}
	var notSet *env.NotSetError
	if !errors.As(err, &notSet) {
		t.Fatalf("error type = %T, want *env.NotSetError", err)
	}
	if notSet.Key != "TEST_REQ_MISSING_XYZ" {
		t.Errorf("Key = %q, want %q", notSet.Key, "TEST_REQ_MISSING_XYZ")
	}
}

func TestRequired_Empty(t *testing.T) {
	t.Setenv("TEST_REQ_EMPTY", "")
	_, err := env.Required("TEST_REQ_EMPTY")
	if err == nil {
		t.Fatal("Required should return error for empty variable")
	}
}
```

Note: the test file needs `"errors"` in its import block. Add it alongside `"testing"` and `"time"`.

**Step 2: Run tests to verify they fail**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestRequired' -v`
Expected: FAIL — `env.Required` undefined

**Step 3: Implement**

Add to `env/env.go` after `Exists`:

```go
// Required returns the value of the environment variable named by key.
// Returns NotSetError if the variable is not set or is empty.
// Use this for variables that must be configured (e.g. DATABASE_URL).
func Required(key string) (string, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return "", &NotSetError{Key: key}
	}
	return v, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestRequired' -v`
Expected: PASS (3 tests)

**Step 5: Full suite — no regressions**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -v`
Expected: All tests PASS

**Step 6: Commit**

```bash
cd /home/krolik/src/go-kit
git add env/env.go env/env_test.go
git commit -m "feat(env): add Required for mandatory env vars"
```

---

### Task 4: IntE, Int64E, FloatE

**Files:**
- Modify: `env/env.go` (add 3 functions after the existing Int/Int64/Float)
- Test: `env/env_test.go` (append tests)

**Step 1: Write tests**

Append to `env/env_test.go`:

```go
func TestIntE_Valid(t *testing.T) {
	t.Setenv("TEST_INTE", "42")
	val, err := env.IntE("TEST_INTE", 0)
	if err != nil {
		t.Fatalf("IntE returned error: %v", err)
	}
	if val != 42 {
		t.Errorf("IntE = %d, want 42", val)
	}
}

func TestIntE_NotSet(t *testing.T) {
	val, err := env.IntE("TEST_INTE_MISSING_XYZ", 99)
	if err != nil {
		t.Fatalf("IntE should not error on unset: %v", err)
	}
	if val != 99 {
		t.Errorf("IntE = %d, want default 99", val)
	}
}

func TestIntE_Invalid(t *testing.T) {
	t.Setenv("TEST_INTE_BAD", "not_a_number")
	_, err := env.IntE("TEST_INTE_BAD", 0)
	if err == nil {
		t.Fatal("IntE should return error for invalid value")
	}
	var pe *env.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want *env.ParseError", err)
	}
	if pe.Key != "TEST_INTE_BAD" || pe.Value != "not_a_number" || pe.Type != "int" {
		t.Errorf("ParseError = {%q, %q, %q}, want {TEST_INTE_BAD, not_a_number, int}", pe.Key, pe.Value, pe.Type)
	}
}

func TestInt64E_Valid(t *testing.T) {
	t.Setenv("TEST_INT64E", "9999999999")
	val, err := env.Int64E("TEST_INT64E", 0)
	if err != nil {
		t.Fatalf("Int64E returned error: %v", err)
	}
	if val != 9999999999 {
		t.Errorf("Int64E = %d, want 9999999999", val)
	}
}

func TestInt64E_Invalid(t *testing.T) {
	t.Setenv("TEST_INT64E_BAD", "xyz")
	_, err := env.Int64E("TEST_INT64E_BAD", 0)
	if err == nil {
		t.Fatal("Int64E should return error for invalid value")
	}
	var pe *env.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want *env.ParseError", err)
	}
	if pe.Type != "int64" {
		t.Errorf("Type = %q, want %q", pe.Type, "int64")
	}
}

func TestFloatE_Valid(t *testing.T) {
	t.Setenv("TEST_FLOATE", "3.14")
	val, err := env.FloatE("TEST_FLOATE", 0)
	if err != nil {
		t.Fatalf("FloatE returned error: %v", err)
	}
	if val != 3.14 {
		t.Errorf("FloatE = %f, want 3.14", val)
	}
}

func TestFloatE_Invalid(t *testing.T) {
	t.Setenv("TEST_FLOATE_BAD", "not_float")
	_, err := env.FloatE("TEST_FLOATE_BAD", 0)
	if err == nil {
		t.Fatal("FloatE should return error for invalid value")
	}
	var pe *env.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want *env.ParseError", err)
	}
	if pe.Type != "float64" {
		t.Errorf("Type = %q, want %q", pe.Type, "float64")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestIntE|TestInt64E|TestFloatE' -v`
Expected: FAIL — functions undefined

**Step 3: Implement**

Add to `env/env.go`. Place each `*E` function directly after its non-error sibling:

After `Int`:
```go
// IntE is like Int but returns a ParseError if the variable is set but not a valid integer.
// If the variable is not set, returns (def, nil).
func IntE(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def, &ParseError{Key: key, Value: v, Type: "int", Err: err}
	}
	return n, nil
}
```

After `Int64`:
```go
// Int64E is like Int64 but returns a ParseError if the variable is set but not a valid int64.
func Int64E(key string, def int64) (int64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def, &ParseError{Key: key, Value: v, Type: "int64", Err: err}
	}
	return n, nil
}
```

After `Float`:
```go
// FloatE is like Float but returns a ParseError if the variable is set but not a valid float64.
func FloatE(key string, def float64) (float64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def, &ParseError{Key: key, Value: v, Type: "float64", Err: err}
	}
	return f, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestIntE|TestInt64E|TestFloatE' -v`
Expected: PASS (7 tests)

**Step 5: Full suite — no regressions**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -v`
Expected: All tests PASS

**Step 6: Commit**

```bash
cd /home/krolik/src/go-kit
git add env/env.go env/env_test.go
git commit -m "feat(env): add IntE, Int64E, FloatE error-returning variants"
```

---

### Task 5: BoolE and DurationE

**Files:**
- Modify: `env/env.go` (add 2 functions after Bool and Duration)
- Test: `env/env_test.go` (append tests)

**Step 1: Write tests**

Append to `env/env_test.go`:

```go
func TestBoolE_Valid(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"TRUE", true},
		{"NO", false},
	} {
		t.Run(tc.val, func(t *testing.T) {
			t.Setenv("TEST_BOOLE", tc.val)
			got, err := env.BoolE("TEST_BOOLE", !tc.want)
			if err != nil {
				t.Fatalf("BoolE(%q) returned error: %v", tc.val, err)
			}
			if got != tc.want {
				t.Errorf("BoolE(%q) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}

func TestBoolE_NotSet(t *testing.T) {
	val, err := env.BoolE("TEST_BOOLE_MISSING_XYZ", true)
	if err != nil {
		t.Fatalf("BoolE should not error on unset: %v", err)
	}
	if val != true {
		t.Errorf("BoolE = %v, want default true", val)
	}
}

func TestBoolE_Invalid(t *testing.T) {
	t.Setenv("TEST_BOOLE_BAD", "maybe")
	_, err := env.BoolE("TEST_BOOLE_BAD", false)
	if err == nil {
		t.Fatal("BoolE should return error for invalid value")
	}
	var pe *env.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want *env.ParseError", err)
	}
	if pe.Key != "TEST_BOOLE_BAD" || pe.Value != "maybe" || pe.Type != "bool" {
		t.Errorf("ParseError = {%q, %q, %q}", pe.Key, pe.Value, pe.Type)
	}
}

func TestDurationE_GoFormat(t *testing.T) {
	t.Setenv("TEST_DURE", "5s")
	val, err := env.DurationE("TEST_DURE", 0)
	if err != nil {
		t.Fatalf("DurationE returned error: %v", err)
	}
	if val != 5*time.Second {
		t.Errorf("DurationE = %v, want 5s", val)
	}
}

func TestDurationE_FloatSeconds(t *testing.T) {
	t.Setenv("TEST_DURE_FLOAT", "3.5")
	val, err := env.DurationE("TEST_DURE_FLOAT", 0)
	if err != nil {
		t.Fatalf("DurationE returned error: %v", err)
	}
	if val != 3500*time.Millisecond {
		t.Errorf("DurationE = %v, want 3.5s", val)
	}
}

func TestDurationE_NotSet(t *testing.T) {
	val, err := env.DurationE("TEST_DURE_MISSING_XYZ", 10*time.Second)
	if err != nil {
		t.Fatalf("DurationE should not error on unset: %v", err)
	}
	if val != 10*time.Second {
		t.Errorf("DurationE = %v, want 10s", val)
	}
}

func TestDurationE_Invalid(t *testing.T) {
	t.Setenv("TEST_DURE_BAD", "not_a_duration")
	_, err := env.DurationE("TEST_DURE_BAD", 0)
	if err == nil {
		t.Fatal("DurationE should return error for invalid value")
	}
	var pe *env.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want *env.ParseError", err)
	}
	if pe.Type != "duration" {
		t.Errorf("Type = %q, want %q", pe.Type, "duration")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestBoolE|TestDurationE' -v`
Expected: FAIL — functions undefined

**Step 3: Implement**

After `Bool` in `env/env.go`:
```go
// BoolE is like Bool but returns a ParseError if the variable is set
// to an unrecognized value (not true/1/yes/false/0/no).
func BoolE(key string, def bool) (bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return def, &ParseError{Key: key, Value: v, Type: "bool"}
	}
}
```

After `Duration` in `env/env.go`:
```go
// DurationE is like Duration but returns a ParseError if the variable is set
// but cannot be parsed. Accepts Go duration strings ("5s", "100ms", "2m30s")
// and falls back to float seconds ("3.5" → 3.5s) for backward compatibility.
func DurationE(key string, def time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	// Try Go duration format first ("5s", "100ms", "2m30s").
	if d, err := time.ParseDuration(v); err == nil {
		return d, nil
	}
	// Fall back to float seconds for backward compat ("3.5" → 3.5s).
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		return time.Duration(secs * float64(time.Second)), nil
	}
	return def, &ParseError{Key: key, Value: v, Type: "duration"}
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestBoolE|TestDurationE' -v`
Expected: PASS (all subtests)

**Step 5: Full suite — no regressions**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -v`
Expected: All tests PASS

**Step 6: Commit**

```bash
cd /home/krolik/src/go-kit
git add env/env.go env/env_test.go
git commit -m "feat(env): add BoolE, DurationE error-returning variants

DurationE also accepts Go duration format (5s, 100ms) in addition to float seconds."
```

---

### Task 6: Must* Variants

**Files:**
- Create: `env/must.go` (all Must functions in one file)
- Test: `env/must_test.go`

**Step 1: Write tests**

```go
// env/must_test.go
package env_test

import (
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/env"
)

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s should have panicked", name)
		}
	}()
	fn()
}

func TestMustRequired_Set(t *testing.T) {
	t.Setenv("TEST_MUSTREQ", "value")
	if got := env.MustRequired("TEST_MUSTREQ"); got != "value" {
		t.Errorf("MustRequired = %q, want %q", got, "value")
	}
}

func TestMustRequired_Panics(t *testing.T) {
	mustPanic(t, "MustRequired", func() {
		env.MustRequired("TEST_MUSTREQ_MISSING_XYZ")
	})
}

func TestMustInt_Valid(t *testing.T) {
	t.Setenv("TEST_MUSTINT", "42")
	if got := env.MustInt("TEST_MUSTINT", 0); got != 42 {
		t.Errorf("MustInt = %d, want 42", got)
	}
}

func TestMustInt_NotSet(t *testing.T) {
	if got := env.MustInt("TEST_MUSTINT_MISSING_XYZ", 99); got != 99 {
		t.Errorf("MustInt = %d, want default 99", got)
	}
}

func TestMustInt_Panics(t *testing.T) {
	t.Setenv("TEST_MUSTINT_BAD", "abc")
	mustPanic(t, "MustInt", func() {
		env.MustInt("TEST_MUSTINT_BAD", 0)
	})
}

func TestMustInt64_Panics(t *testing.T) {
	t.Setenv("TEST_MUSTINT64_BAD", "abc")
	mustPanic(t, "MustInt64", func() {
		env.MustInt64("TEST_MUSTINT64_BAD", 0)
	})
}

func TestMustFloat_Panics(t *testing.T) {
	t.Setenv("TEST_MUSTFLOAT_BAD", "abc")
	mustPanic(t, "MustFloat", func() {
		env.MustFloat("TEST_MUSTFLOAT_BAD", 0)
	})
}

func TestMustBool_Panics(t *testing.T) {
	t.Setenv("TEST_MUSTBOOL_BAD", "maybe")
	mustPanic(t, "MustBool", func() {
		env.MustBool("TEST_MUSTBOOL_BAD", false)
	})
}

func TestMustDuration_Panics(t *testing.T) {
	t.Setenv("TEST_MUSTDUR_BAD", "xyz")
	mustPanic(t, "MustDuration", func() {
		env.MustDuration("TEST_MUSTDUR_BAD", time.Second)
	})
}

func TestMustDuration_Valid(t *testing.T) {
	t.Setenv("TEST_MUSTDUR", "5s")
	if got := env.MustDuration("TEST_MUSTDUR", 0); got != 5*time.Second {
		t.Errorf("MustDuration = %v, want 5s", got)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestMust' -v`
Expected: FAIL — functions undefined

**Step 3: Implement**

```go
// env/must.go
package env

import "time"

// MustRequired returns the value of the environment variable named by key.
// Panics if the variable is not set or empty. Intended for fail-fast startup validation.
func MustRequired(key string) string {
	v, err := Required(key)
	if err != nil {
		panic(err)
	}
	return v
}

// MustInt is like Int but panics if the variable is set to an invalid integer.
func MustInt(key string, def int) int {
	v, err := IntE(key, def)
	if err != nil {
		panic(err)
	}
	return v
}

// MustInt64 is like Int64 but panics if the variable is set to an invalid int64.
func MustInt64(key string, def int64) int64 {
	v, err := Int64E(key, def)
	if err != nil {
		panic(err)
	}
	return v
}

// MustFloat is like Float but panics if the variable is set to an invalid float64.
func MustFloat(key string, def float64) float64 {
	v, err := FloatE(key, def)
	if err != nil {
		panic(err)
	}
	return v
}

// MustBool is like Bool but panics if the variable is set to an unrecognized value.
func MustBool(key string, def bool) bool {
	v, err := BoolE(key, def)
	if err != nil {
		panic(err)
	}
	return v
}

// MustDuration is like Duration but panics if the variable is set to an invalid duration.
func MustDuration(key string, def time.Duration) time.Duration {
	v, err := DurationE(key, def)
	if err != nil {
		panic(err)
	}
	return v
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestMust' -v`
Expected: PASS (10 tests)

**Step 5: Full suite — no regressions**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -v`
Expected: All tests PASS

**Step 6: Commit**

```bash
cd /home/krolik/src/go-kit
git add env/must.go env/must_test.go
git commit -m "feat(env): add Must* panic variants for fail-fast startup validation"
```

---

### Task 7: Update Duration to Accept Go Format (backport DurationE logic)

The existing `Duration` function only parses float-seconds. Since `DurationE` already accepts Go duration format ("5s", "100ms"), update `Duration` to match — keeping backward compatibility.

**Files:**
- Modify: `env/env.go` (update `Duration` function)
- Modify: `env/env_test.go` (add test for new format)

**Step 1: Write test for Go duration format in existing Duration**

Append to `env/env_test.go`:

```go
func TestDuration_GoFormat(t *testing.T) {
	t.Setenv("TEST_DUR_GO", "5s")
	want := 5 * time.Second
	if got := env.Duration("TEST_DUR_GO", 0); got != want {
		t.Errorf("Duration = %v, want %v", got, want)
	}
}

func TestDuration_GoFormatComplex(t *testing.T) {
	t.Setenv("TEST_DUR_GO2", "2m30s")
	want := 2*time.Minute + 30*time.Second
	if got := env.Duration("TEST_DUR_GO2", 0); got != want {
		t.Errorf("Duration = %v, want %v", got, want)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestDuration_GoFormat' -v`
Expected: FAIL — "5s" is not a valid float, returns default 0

**Step 3: Update Duration**

Replace the `Duration` function body in `env/env.go`:

```go
// Duration returns the environment variable parsed as a duration.
// Accepts Go duration strings ("5s", "100ms", "2m30s") and float seconds ("3.5" → 3.5s).
// Returns def if not set or invalid.
func Duration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	// Try Go duration format first.
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	// Fall back to float seconds for backward compat.
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		return time.Duration(secs * float64(time.Second))
	}
	return def
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -run 'TestDuration' -v`
Expected: PASS — all Duration tests including the old float format "3.5"

**Step 5: Full suite — no regressions**

Run: `cd /home/krolik/src/go-kit && go test ./env/ -v`
Expected: All tests PASS

**Step 6: Lint**

Run: `cd /home/krolik/src/go-kit && golangci-lint run ./env/`
Expected: No errors (or only pre-existing ones)

**Step 7: Commit**

```bash
cd /home/krolik/src/go-kit
git add env/env.go env/env_test.go
git commit -m "feat(env): Duration now accepts Go format (5s, 100ms, 2m30s)

Falls back to float seconds for backward compatibility."
```

---

### Task 8: Update README

**Files:**
- Modify: `README.md` (update env section with new functions)

**Step 1: Update the env section in README.md**

Add the new functions to the env documentation section. After the existing `env` examples, add:

```markdown
#### Error handling

```go
// Error-returning variants — return ParseError on invalid values
port, err := env.IntE("PORT", 8080)        // err if PORT="abc"
debug, err := env.BoolE("DEBUG", false)     // err if DEBUG="maybe"
timeout, err := env.DurationE("TIMEOUT", 30*time.Second) // accepts "5s", "100ms", "2m30s"

// Required — must be set, returns NotSetError if missing
dbURL, err := env.Required("DATABASE_URL")

// Lookup — distinguish "not set" from "set to empty"
val, ok := env.Lookup("OPTIONAL_VAR")

// Must* — panic on invalid (for fail-fast main() init)
dbURL := env.MustRequired("DATABASE_URL")
port := env.MustInt("PORT", 8080)
```

**Step 2: Commit**

```bash
cd /home/krolik/src/go-kit
git add README.md
git commit -m "docs: update README with env error handling functions"
```

---

## Summary

| Task | What | New functions | Tests |
|------|------|---------------|-------|
| 1 | Error types | `NotSetError`, `ParseError` | 4 |
| 2 | Lookup primitives | `Lookup`, `Exists` | 6 |
| 3 | Required validation | `Required` | 3 |
| 4 | Numeric error variants | `IntE`, `Int64E`, `FloatE` | 7 |
| 5 | Bool/Duration error variants | `BoolE`, `DurationE` | 11 |
| 6 | Panic variants | `MustRequired`, `MustInt`, `MustInt64`, `MustFloat`, `MustBool`, `MustDuration` | 10 |
| 7 | Duration format upgrade | Updated `Duration` | 2 |
| 8 | Documentation | README update | — |
| **Total** | | **17 new functions + 2 error types** | **43 new tests** |
