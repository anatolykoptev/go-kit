package redirectmatch_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/redirectmatch"
)

func TestNormalize(t *testing.T) {
	full := redirectmatch.DefaultPolicy()

	cases := []struct {
		name   string
		input  string
		policy redirectmatch.Policy
		want   string
	}{
		// Basic cases
		{"root stays root", "/", full, "/"},
		{"empty becomes root", "", full, "/"},
		{"lowercase", "/Foo", full, "/foo"},
		{"strip trailing slash", "/foo/", full, "/foo"},
		{"strip trailing slash keeps content", "/Foo/Bar/", full, "/foo/bar"},
		{"already clean", "/foo/bar", full, "/foo/bar"},

		// Policy toggles
		{"no lowercase", "/FOO", redirectmatch.Policy{StripTrailingSlash: true, LowerCase: false, DecodeOnce: true}, "/FOO"},
		{"no strip slash", "/foo/", redirectmatch.Policy{StripTrailingSlash: false, LowerCase: true, DecodeOnce: true}, "/foo/"},
		{"root no strip", "/", redirectmatch.Policy{StripTrailingSlash: true, LowerCase: true, DecodeOnce: true}, "/"},

		// Percent-decode
		{"decode %20", "/foo%20bar", full, "/foo bar"},
		{"decode uppercase hex", "/foo%2Fbar", full, "/foo/bar"},
		{"no double decode", "/foo%2520bar", full, "/foo%20bar"}, // decodes once: %25 -> %, giving /foo%20bar

		// Idempotency check
		{"already normalized", "/foo/bar", full, "/foo/bar"},
		{"double slash", "//foo", full, "//foo"}, // we don't collapse double slashes
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redirectmatch.Normalize(tc.input, tc.policy)
			if got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizeIdempotent(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	inputs := []string{
		"/",
		"/foo",
		"/Foo/Bar/",
		"/foo%20bar",
		"",
		"/hello/world/",
		"/A/B/C/",
	}
	for _, in := range inputs {
		once := redirectmatch.Normalize(in, p)
		twice := redirectmatch.Normalize(once, p)
		if once != twice {
			t.Errorf("Normalize not idempotent for %q: once=%q twice=%q", in, once, twice)
		}
	}
}

func TestDefaultPolicy(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	if !p.StripTrailingSlash {
		t.Error("DefaultPolicy: StripTrailingSlash should be true")
	}
	if !p.LowerCase {
		t.Error("DefaultPolicy: LowerCase should be true")
	}
	if !p.DecodeOnce {
		t.Error("DefaultPolicy: DecodeOnce should be true")
	}
}
