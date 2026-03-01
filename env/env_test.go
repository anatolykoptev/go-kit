package env_test

import (
	"testing"
	"time"

	"github.com/anatolykoptev/go-kit/env"
)

func TestStr(t *testing.T) {
	t.Setenv("TEST_STR", "hello")
	if got := env.Str("TEST_STR", "default"); got != "hello" {
		t.Errorf("Str = %q, want %q", got, "hello")
	}
}

func TestStr_Default(t *testing.T) {
	if got := env.Str("TEST_STR_MISSING", "fallback"); got != "fallback" {
		t.Errorf("Str = %q, want %q", got, "fallback")
	}
}

func TestInt(t *testing.T) {
	t.Setenv("TEST_INT", "42")
	if got := env.Int("TEST_INT", 0); got != 42 {
		t.Errorf("Int = %d, want %d", got, 42)
	}
}

func TestInt_Invalid(t *testing.T) {
	t.Setenv("TEST_INT_BAD", "not_a_number")
	if got := env.Int("TEST_INT_BAD", 99); got != 99 {
		t.Errorf("Int = %d, want default %d", got, 99)
	}
}

func TestInt_Default(t *testing.T) {
	if got := env.Int("TEST_INT_MISSING", 7); got != 7 {
		t.Errorf("Int = %d, want %d", got, 7)
	}
}

func TestInt64(t *testing.T) {
	t.Setenv("TEST_INT64", "9999999999")
	if got := env.Int64("TEST_INT64", 0); got != 9999999999 {
		t.Errorf("Int64 = %d, want %d", got, int64(9999999999))
	}
}

func TestFloat(t *testing.T) {
	t.Setenv("TEST_FLOAT", "3.14")
	if got := env.Float("TEST_FLOAT", 0); got != 3.14 {
		t.Errorf("Float = %f, want %f", got, 3.14)
	}
}

func TestFloat_Default(t *testing.T) {
	if got := env.Float("TEST_FLOAT_MISS", 2.71); got != 2.71 {
		t.Errorf("Float = %f, want %f", got, 2.71)
	}
}

func TestBool(t *testing.T) {
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
	} {
		t.Run(tc.val, func(t *testing.T) {
			t.Setenv("TEST_BOOL", tc.val)
			if got := env.Bool("TEST_BOOL", !tc.want); got != tc.want {
				t.Errorf("Bool(%q) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}

func TestBool_Default(t *testing.T) {
	if got := env.Bool("TEST_BOOL_MISS", true); got != true {
		t.Errorf("Bool = %v, want true", got)
	}
}

func TestDuration(t *testing.T) {
	t.Setenv("TEST_DUR", "3.5")
	want := 3500 * time.Millisecond
	if got := env.Duration("TEST_DUR", 0); got != want {
		t.Errorf("Duration = %v, want %v", got, want)
	}
}

func TestDuration_Default(t *testing.T) {
	want := 10 * time.Second
	if got := env.Duration("TEST_DUR_MISS", want); got != want {
		t.Errorf("Duration = %v, want %v", got, want)
	}
}

func TestList(t *testing.T) {
	t.Setenv("TEST_LIST", "a, b ,c,,d")
	got := env.List("TEST_LIST", "")
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("List len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestList_Empty(t *testing.T) {
	got := env.List("TEST_LIST_MISS", "")
	if got != nil {
		t.Errorf("List = %v, want nil", got)
	}
}

func TestList_Default(t *testing.T) {
	got := env.List("TEST_LIST_MISS", "x,y")
	want := []string{"x", "y"}
	if len(got) != len(want) {
		t.Fatalf("List len = %d, want %d", len(got), len(want))
	}
}

func TestInt64List(t *testing.T) {
	t.Setenv("TEST_INT64LIST", "1, 2, 3, bad, 4")
	got := env.Int64List("TEST_INT64LIST")
	want := []int64{1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("Int64List len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Int64List[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestInt64List_Empty(t *testing.T) {
	got := env.Int64List("TEST_INT64LIST_MISS")
	if got != nil {
		t.Errorf("Int64List = %v, want nil", got)
	}
}

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
