# env Phase C: Source Interface, File, Expand, Binary Data

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add testability via Source interface, Docker secrets via File, variable expansion via Expand, and binary data decoding via Base64/Hex — completing the env package for production use.

**Architecture:** Source interface decouples from os.Getenv. All existing functions refactored to use DefaultSource (backward-compatible via OSSource default). MapSource enables parallel-safe tests. New functions added to env.go. stdlib only.

**Tech Stack:** Go stdlib only (`encoding/base64`, `encoding/hex`, `os`)

---

### Task 1: All env code additions

**Files:** env/env.go

#### 1a. Add Source interface, OSSource, DefaultSource, MapSource, getenv helper

Add after the import block (before Lookup):

```go
// Source provides environment variable lookups. Replace DefaultSource
// for testing or to read from alternative sources.
type Source interface {
	Lookup(key string) (string, bool)
}

type osSource struct{}

func (osSource) Lookup(key string) (string, bool) { return os.LookupEnv(key) }

// DefaultSource is the global source for all env functions.
// Defaults to OSSource (reads from os.LookupEnv).
// Replace with MapSource in tests for parallel-safe, isolated testing.
var DefaultSource Source = osSource{}

type mapSource map[string]string

// MapSource returns a Source backed by a map. Use in tests:
//
//	env.DefaultSource = env.MapSource(map[string]string{"KEY": "value"})
func MapSource(m map[string]string) Source {
	return mapSource(m)
}

func (ms mapSource) Lookup(key string) (string, bool) {
	v, ok := ms[key]
	return v, ok
}

// getenv returns the value from DefaultSource, or "" if not set.
func getenv(key string) string {
	v, _ := DefaultSource.Lookup(key)
	return v
}
```

#### 1b. Refactor all existing functions to use DefaultSource

Replace ALL `os.LookupEnv(key)` → `DefaultSource.Lookup(key)` (use replace_all).
Replace ALL `os.Getenv(key)` → `getenv(key)` (use replace_all).

This affects: Lookup, Exists, Required, Str, Int, IntE, Int64, Int64E, Float, FloatE, Uint, UintE, Uint64, Uint64E, Bool, BoolE, Duration, DurationE, List (via Str), Int64List, Map (via Str), URL (via Str), URLE.

After replacement, `"os"` import is still needed for os.ReadFile and os.Expand (added below).

#### 1c. Add File and FileE

```go
// File reads a file path from the environment variable and returns its contents.
// Trailing newlines are trimmed. Returns def if not set or file cannot be read.
// Useful for Docker secrets (/run/secrets/) and Kubernetes secret volumes.
func File(key, def string) string {
	path := getenv(key)
	if path == "" {
		return def
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	return strings.TrimRight(string(data), "\n")
}

// FileE is like File but returns an error if the variable is not set
// or the file cannot be read.
func FileE(key string) (string, error) {
	v, ok := DefaultSource.Lookup(key)
	if !ok || v == "" {
		return "", &NotSetError{Key: key}
	}
	data, err := os.ReadFile(v)
	if err != nil {
		return "", &ParseError{Key: key, Value: v, Type: "file", Err: err}
	}
	return strings.TrimRight(string(data), "\n"), nil
}
```

#### 1d. Add Expand

```go
// Expand returns the environment variable with ${VAR} references expanded.
// Uses os.Expand with the current DefaultSource for variable resolution.
// Returns def if the variable is not set.
func Expand(key, def string) string {
	v := Str(key, def)
	if v == "" {
		return ""
	}
	return os.Expand(v, getenv)
}
```

#### 1e. Add Base64 and Base64E

Add `"encoding/base64"` to imports.

```go
// Base64 returns the environment variable decoded from standard base64.
// Returns def if not set or invalid.
func Base64(key string, def []byte) []byte {
	v := getenv(key)
	if v == "" {
		return def
	}
	data, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return def
	}
	return data
}

// Base64E is like Base64 but returns a ParseError on invalid base64.
func Base64E(key string, def []byte) ([]byte, error) {
	v, ok := DefaultSource.Lookup(key)
	if !ok || v == "" {
		return def, nil
	}
	data, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return def, &ParseError{Key: key, Value: v, Type: "base64", Err: err}
	}
	return data, nil
}
```

#### 1f. Add Hex and HexE

Add `"encoding/hex"` to imports.

```go
// Hex returns the environment variable decoded from hex.
// Returns def if not set or invalid.
func Hex(key string, def []byte) []byte {
	v := getenv(key)
	if v == "" {
		return def
	}
	data, err := hex.DecodeString(v)
	if err != nil {
		return def
	}
	return data
}

// HexE is like Hex but returns a ParseError on invalid hex.
func HexE(key string, def []byte) ([]byte, error) {
	v, ok := DefaultSource.Lookup(key)
	if !ok || v == "" {
		return def, nil
	}
	data, err := hex.DecodeString(v)
	if err != nil {
		return def, &ParseError{Key: key, Value: v, Type: "hex", Err: err}
	}
	return data, nil
}
```

**Step 1:** Apply all changes (1a-1f).

**Step 2:** Run existing tests:
```bash
cd /home/krolik/src/go-kit && go test ./env/ -v -count=1
```
Expected: All existing tests PASS (DefaultSource is OSSource by default, backward-compatible).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add env/env.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "env: add Source interface, File, Expand, Base64, Hex

Phase C power features:
- Source interface: decouple from os.Getenv for testability
- MapSource: parallel-safe testing without real env vars
- File/FileE: read Docker secrets / Kubernetes volumes
- Expand: resolve \${VAR} references via os.Expand
- Base64/Base64E, Hex/HexE: binary data from env vars
- All existing functions refactored to use DefaultSource

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Tests for all new features

**Files:** env/env_test.go

Add `"os"` to imports (for os.CreateTemp, os.Remove, os.WriteFile).

**Test: MapSource basic**

```go
func TestMapSource(t *testing.T) {
	orig := env.DefaultSource
	t.Cleanup(func() { env.DefaultSource = orig })

	env.DefaultSource = env.MapSource(map[string]string{
		"KEY":  "value",
		"PORT": "8080",
	})

	if got := env.Str("KEY", ""); got != "value" {
		t.Errorf("Str = %q, want %q", got, "value")
	}
	if got := env.Int("PORT", 0); got != 8080 {
		t.Errorf("Int = %d, want 8080", got)
	}
	if got := env.Str("MISSING", "def"); got != "def" {
		t.Errorf("Str = %q, want %q", got, "def")
	}
}
```

**Test: MapSource with Lookup**

```go
func TestMapSource_Lookup(t *testing.T) {
	orig := env.DefaultSource
	t.Cleanup(func() { env.DefaultSource = orig })

	env.DefaultSource = env.MapSource(map[string]string{
		"EMPTY": "",
	})

	v, ok := env.Lookup("EMPTY")
	if !ok || v != "" {
		t.Errorf("Lookup = (%q, %v), want (%q, true)", v, ok, "")
	}
	_, ok = env.Lookup("NOPE")
	if ok {
		t.Error("Lookup should return false for missing key")
	}
}
```

**Test: File reads file contents**

```go
func TestFile(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/secret"
	if err := os.WriteFile(path, []byte("secret_value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SECRET_FILE", path)
	got := env.File("SECRET_FILE", "")
	if got != "secret_value" {
		t.Errorf("File = %q, want %q", got, "secret_value")
	}
}
```

**Test: File default when not set**

```go
func TestFile_Default(t *testing.T) {
	got := env.File("FILE_MISSING_XYZ", "fallback")
	if got != "fallback" {
		t.Errorf("File = %q, want %q", got, "fallback")
	}
}
```

**Test: FileE error when not set**

```go
func TestFileE_NotSet(t *testing.T) {
	_, err := env.FileE("FILE_E_MISSING_XYZ")
	if err == nil {
		t.Fatal("FileE should return error when not set")
	}
	var notSet *env.NotSetError
	if !errors.As(err, &notSet) {
		t.Fatalf("error type = %T, want *NotSetError", err)
	}
}
```

**Test: Expand resolves variables**

```go
func TestExpand(t *testing.T) {
	t.Setenv("HOST", "localhost")
	t.Setenv("PORT", "5432")
	t.Setenv("DB_URL", "postgres://${HOST}:${PORT}/mydb")

	got := env.Expand("DB_URL", "")
	want := "postgres://localhost:5432/mydb"
	if got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
}
```

**Test: Expand default when not set**

```go
func TestExpand_Default(t *testing.T) {
	t.Setenv("EXPAND_HOST", "example.com")
	got := env.Expand("EXPAND_MISSING_XYZ", "https://${EXPAND_HOST}")
	if got != "https://example.com" {
		t.Errorf("Expand = %q, want %q", got, "https://example.com")
	}
}
```

**Test: Base64 decode**

```go
func TestBase64(t *testing.T) {
	// "hello world" = "aGVsbG8gd29ybGQ="
	t.Setenv("B64_DATA", "aGVsbG8gd29ybGQ=")
	got := env.Base64("B64_DATA", nil)
	if string(got) != "hello world" {
		t.Errorf("Base64 = %q, want %q", got, "hello world")
	}
}
```

**Test: Base64E invalid**

```go
func TestBase64E_Invalid(t *testing.T) {
	t.Setenv("B64_BAD", "not-valid-base64!!!")
	_, err := env.Base64E("B64_BAD", nil)
	if err == nil {
		t.Fatal("Base64E should return error for invalid base64")
	}
	var pe *env.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want *ParseError", err)
	}
	if pe.Type != "base64" {
		t.Errorf("Type = %q, want %q", pe.Type, "base64")
	}
}
```

**Test: Hex decode**

```go
func TestHex(t *testing.T) {
	// "hello" = "68656c6c6f"
	t.Setenv("HEX_DATA", "68656c6c6f")
	got := env.Hex("HEX_DATA", nil)
	if string(got) != "hello" {
		t.Errorf("Hex = %q, want %q", got, "hello")
	}
}
```

**Test: HexE invalid**

```go
func TestHexE_Invalid(t *testing.T) {
	t.Setenv("HEX_BAD", "xyz")
	_, err := env.HexE("HEX_BAD", nil)
	if err == nil {
		t.Fatal("HexE should return error for invalid hex")
	}
	var pe *env.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want *ParseError", err)
	}
	if pe.Type != "hex" {
		t.Errorf("Type = %q, want %q", pe.Type, "hex")
	}
}
```

**Step 1:** Add all 11 tests to env_test.go.

**Step 2:** Run all tests:
```bash
cd /home/krolik/src/go-kit && go test ./env/ -v -count=1
```
Expected: All tests PASS (existing + 11 new).

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add env/env_test.go
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "env: add tests for Source, File, Expand, Base64, Hex

11 new tests: MapSource (basic/lookup), File (read/default),
FileE (not-set), Expand (resolve/default), Base64, Base64E,
Hex, HexE.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Update README and ROADMAP

**Files:** README.md, docs/ROADMAP.md

**README changes** — update env section with new features:

```go
import "github.com/anatolykoptev/go-kit/env"

port := env.Int("PORT", 8080)
debug := env.Bool("DEBUG", false)

// Docker secrets / Kubernetes volumes
dbPass := env.File("DB_PASSWORD_FILE", "")

// Variable expansion
dbURL := env.Expand("DATABASE_URL", "postgres://localhost:5432/mydb")

// Binary data
cert := env.Base64("TLS_CERT", nil)
key := env.Hex("API_KEY_HEX", nil)

// Testability — decouple from os.Getenv
env.DefaultSource = env.MapSource(map[string]string{
    "PORT": "9090",
})
```

Update bullet points:
- Source interface for testability (MapSource for parallel-safe tests)
- File: read Docker secrets and Kubernetes volumes
- Expand: resolve ${VAR} references
- Base64/Hex: binary data from env vars

**ROADMAP changes:**
- Mark env Phase C as DONE

**Step 1:** Update README.md env section.

**Step 2:** Update ROADMAP.md env Phase C status.

**Step 3:** Commit:
```bash
sudo -u krolik git -C /home/krolik/src/go-kit add README.md docs/ROADMAP.md
sudo -u krolik git -C /home/krolik/src/go-kit commit -m "docs: update env section for Phase C features

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```
