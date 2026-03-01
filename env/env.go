// Package env provides typed access to environment variables with defaults.
// Zero external dependencies. Designed to replace duplicated env/envInt/envList
// helpers found across go-* services.
package env

import (
	"os"
	"strconv"
	"strings"
	"time"
)

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

// Str returns the value of the environment variable named by key,
// or def if the variable is not set or empty.
func Str(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Int returns the environment variable as an int, or def if not set or invalid.
func Int(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// Int64 returns the environment variable as int64, or def if not set or invalid.
func Int64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}

// Float returns the environment variable as float64, or def if not set or invalid.
func Float(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// Bool returns the environment variable as a bool, or def if not set.
// Truthy: "true", "1", "yes" (case-insensitive).
// Falsy: "false", "0", "no" (case-insensitive).
// Anything else returns def.
func Bool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return def
	}
}

// Duration returns the environment variable parsed as seconds (float),
// or def if not set or invalid. E.g. "3.5" → 3.5s.
func Duration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if secs, err := strconv.ParseFloat(v, 64); err == nil {
			return time.Duration(secs * float64(time.Second))
		}
	}
	return def
}

// List returns a comma-separated environment variable as a trimmed string slice.
// Empty entries are dropped. Returns nil if the variable is not set and def is "".
func List(key, def string) []string {
	v := Str(key, def)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Int64List returns a comma-separated list of int64 values.
// Non-numeric entries are silently skipped. Returns nil if not set.
func Int64List(key string) []int64 {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var result []int64
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			result = append(result, n)
		}
	}
	return result
}
