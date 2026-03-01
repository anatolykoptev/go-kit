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
