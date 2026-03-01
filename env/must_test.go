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
