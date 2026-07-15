package redirectmatch_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/redirectmatch"
)

func TestCompile_ValidCases(t *testing.T) {
	cases := []struct {
		name string
		spec redirectmatch.RuleSpec
	}{
		{
			"exact 301",
			redirectmatch.RuleSpec{ID: 1, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/bar", StatusCode: 301},
		},
		{
			"exact 302",
			redirectmatch.RuleSpec{ID: 2, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/bar", StatusCode: 302},
		},
		{
			"prefix 307",
			redirectmatch.RuleSpec{ID: 3, SourcePath: "/old", MatchType: redirectmatch.Prefix, Target: "/new", StatusCode: 307},
		},
		{
			"regex 308 with capture",
			redirectmatch.RuleSpec{ID: 4, SourcePath: `^/blog/(\d+)$`, MatchType: redirectmatch.Regex, Target: "/posts/$1", StatusCode: 308},
		},
		{
			"410 gone with empty target",
			redirectmatch.RuleSpec{ID: 5, SourcePath: "/deleted", MatchType: redirectmatch.Exact, Target: "", StatusCode: 410},
		},
		{
			"451 legal with empty target",
			redirectmatch.RuleSpec{ID: 6, SourcePath: "/legal", MatchType: redirectmatch.Exact, Target: "", StatusCode: 451},
		},
		{
			"regex with $1 and $2",
			redirectmatch.RuleSpec{ID: 7, SourcePath: `^/(\w+)/(\d+)$`, MatchType: redirectmatch.Regex, Target: "/$2/$1", StatusCode: 301},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule, err := redirectmatch.Compile(tc.spec)
			if err != nil {
				t.Fatalf("Compile(%v) unexpected error: %v", tc.spec, err)
			}
			if rule.ID != tc.spec.ID {
				t.Errorf("rule.ID = %d, want %d", rule.ID, tc.spec.ID)
			}
			if rule.StatusCode != tc.spec.StatusCode {
				t.Errorf("rule.StatusCode = %d, want %d", rule.StatusCode, tc.spec.StatusCode)
			}
		})
	}
}

func TestCompile_InvalidStatusCode(t *testing.T) {
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/bar", StatusCode: 200}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject status code 200")
	}
}

func TestCompile_3xxEmptyTarget(t *testing.T) {
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "", StatusCode: 301}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject 301 with empty target")
	}
}

func TestCompile_410WithNonEmptyTarget(t *testing.T) {
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/bar", StatusCode: 410}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject 410 with non-empty target")
	}
}

func TestCompile_451WithNonEmptyTarget(t *testing.T) {
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/bar", StatusCode: 451}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject 451 with non-empty target")
	}
}

func TestCompile_SelfRedirect(t *testing.T) {
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/foo", StatusCode: 301}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject self-redirect (Target == SourcePath)")
	}
}

func TestCompile_BadRegex(t *testing.T) {
	// Backreference \1 is not valid RE2
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: `(a)\1`, MatchType: redirectmatch.Regex, Target: "/bar", StatusCode: 301}
	_, err := redirectmatch.Compile(spec)
	if err == nil {
		t.Error("Compile should reject un-RE2 regex with backreference")
	}
}

func TestCompile_RegexWithCapture(t *testing.T) {
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: `^/old/(\d+)$`, MatchType: redirectmatch.Regex, Target: "/new/$1", StatusCode: 301}
	rule, err := redirectmatch.Compile(spec)
	if err != nil {
		t.Fatalf("Compile should accept valid regex with $1: %v", err)
	}
	if rule.MatchType != redirectmatch.Regex {
		t.Errorf("rule.MatchType = %q, want Regex", rule.MatchType)
	}
}
