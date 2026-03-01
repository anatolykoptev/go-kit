package env_test

import (
	"errors"
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

func TestUint(t *testing.T) {
	t.Setenv("TEST_UINT", "42")
	if got := env.Uint("TEST_UINT", 0); got != 42 {
		t.Errorf("Uint = %d, want 42", got)
	}
}

func TestUint_Default(t *testing.T) {
	if got := env.Uint("TEST_UINT_MISSING", 7); got != 7 {
		t.Errorf("Uint = %d, want 7", got)
	}
}

func TestUint_Invalid(t *testing.T) {
	t.Setenv("TEST_UINT_BAD", "-5")
	if got := env.Uint("TEST_UINT_BAD", 99); got != 99 {
		t.Errorf("Uint = %d, want default 99", got)
	}
}

func TestUintE_Valid(t *testing.T) {
	t.Setenv("TEST_UINTE", "100")
	val, err := env.UintE("TEST_UINTE", 0)
	if err != nil {
		t.Fatalf("UintE error: %v", err)
	}
	if val != 100 {
		t.Errorf("UintE = %d, want 100", val)
	}
}

func TestUintE_Invalid(t *testing.T) {
	t.Setenv("TEST_UINTE_BAD", "-1")
	_, err := env.UintE("TEST_UINTE_BAD", 0)
	if err == nil {
		t.Fatal("UintE should error on negative")
	}
	var pe *env.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want *ParseError", err)
	}
	if pe.Type != "uint" {
		t.Errorf("Type = %q, want %q", pe.Type, "uint")
	}
}

func TestUint64(t *testing.T) {
	t.Setenv("TEST_UINT64", "18446744073709551615")
	if got := env.Uint64("TEST_UINT64", 0); got != 18446744073709551615 {
		t.Errorf("Uint64 = %d, want max uint64", got)
	}
}

func TestUint64E_Invalid(t *testing.T) {
	t.Setenv("TEST_UINT64E_BAD", "abc")
	_, err := env.Uint64E("TEST_UINT64E_BAD", 0)
	if err == nil {
		t.Fatal("Uint64E should error on invalid")
	}
	var pe *env.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("error type = %T, want *ParseError", err)
	}
	if pe.Type != "uint64" {
		t.Errorf("Type = %q, want %q", pe.Type, "uint64")
	}
}

func TestMap(t *testing.T) {
	t.Setenv("TEST_MAP", "Content-Type:json, Accept:*/*")
	got := env.Map("TEST_MAP", "")
	if got == nil {
		t.Fatal("Map returned nil")
	}
	if got["Content-Type"] != "json" {
		t.Errorf("Content-Type = %q, want %q", got["Content-Type"], "json")
	}
	if got["Accept"] != "*/*" {
		t.Errorf("Accept = %q, want %q", got["Accept"], "*/*")
	}
}

func TestMap_Default(t *testing.T) {
	got := env.Map("TEST_MAP_MISSING", "a:1,b:2")
	if got == nil {
		t.Fatal("Map returned nil with default")
	}
	if got["a"] != "1" || got["b"] != "2" {
		t.Errorf("Map = %v, want {a:1 b:2}", got)
	}
}

func TestMap_Empty(t *testing.T) {
	got := env.Map("TEST_MAP_MISSING2", "")
	if got != nil {
		t.Errorf("Map = %v, want nil", got)
	}
}

func TestMap_SkipsBadEntries(t *testing.T) {
	t.Setenv("TEST_MAP_BAD", "good:value,nocolon,also:works")
	got := env.Map("TEST_MAP_BAD", "")
	if len(got) != 2 {
		t.Fatalf("Map len = %d, want 2: %v", len(got), got)
	}
	if got["good"] != "value" || got["also"] != "works" {
		t.Errorf("Map = %v", got)
	}
}

func TestMap_EmptyValue(t *testing.T) {
	t.Setenv("TEST_MAP_EVAL", "key:")
	got := env.Map("TEST_MAP_EVAL", "")
	if got == nil {
		t.Fatal("Map returned nil")
	}
	if got["key"] != "" {
		t.Errorf("key value = %q, want empty", got["key"])
	}
}

func TestURL(t *testing.T) {
	t.Setenv("TEST_URL", "https://example.com/path?q=1")
	got := env.URL("TEST_URL", "")
	if got == nil {
		t.Fatal("URL returned nil")
	}
	if got.Host != "example.com" {
		t.Errorf("Host = %q, want %q", got.Host, "example.com")
	}
	if got.Path != "/path" {
		t.Errorf("Path = %q, want %q", got.Path, "/path")
	}
}

func TestURL_Default(t *testing.T) {
	got := env.URL("TEST_URL_MISSING", "https://default.com")
	if got == nil {
		t.Fatal("URL returned nil")
	}
	if got.Host != "default.com" {
		t.Errorf("Host = %q, want %q", got.Host, "default.com")
	}
}

func TestURL_Empty(t *testing.T) {
	got := env.URL("TEST_URL_MISSING2", "")
	if got != nil {
		t.Errorf("URL = %v, want nil", got)
	}
}

func TestURLE_Valid(t *testing.T) {
	t.Setenv("TEST_URLE", "https://api.example.com/v1")
	u, err := env.URLE("TEST_URLE", "")
	if err != nil {
		t.Fatalf("URLE error: %v", err)
	}
	if u.Host != "api.example.com" {
		t.Errorf("Host = %q, want %q", u.Host, "api.example.com")
	}
}

func TestURLE_NotSet(t *testing.T) {
	u, err := env.URLE("TEST_URLE_MISSING", "https://fallback.com")
	if err != nil {
		t.Fatalf("URLE error: %v", err)
	}
	if u.Host != "fallback.com" {
		t.Errorf("Host = %q, want %q", u.Host, "fallback.com")
	}
}

func TestURLE_Empty(t *testing.T) {
	u, err := env.URLE("TEST_URLE_EMPTY", "")
	if err != nil {
		t.Fatalf("URLE error: %v", err)
	}
	if u != nil {
		t.Errorf("URLE = %v, want nil", u)
	}
}
