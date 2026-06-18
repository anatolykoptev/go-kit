package redirectmatch_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/redirectmatch"
)

func TestBuildRuleSet_GoodAndBad(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: "/good", MatchType: redirectmatch.Exact, Target: "/ok", StatusCode: 301},
		{ID: 2, SourcePath: "/bad", MatchType: redirectmatch.Exact, Target: "", StatusCode: 301}, // bad: 301 with empty target
	}
	set, errs := redirectmatch.BuildRuleSet(specs, p)
	if len(errs) != 1 {
		t.Fatalf("expected 1 CompileError, got %d: %v", len(errs), errs)
	}
	if errs[0].ID != 2 {
		t.Errorf("CompileError.ID = %d, want 2", errs[0].ID)
	}

	// The good rule should still be in the set
	dec := redirectmatch.Resolve(set, "/good", "")
	if !dec.Matched {
		t.Error("good rule should still resolve after bad rule excluded")
	}
}

func TestBuildRuleSet_InvalidSpecDoesNotPoison(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: "(a)\\1", MatchType: redirectmatch.Regex, Target: "/ok", StatusCode: 301}, // bad regex
		{ID: 2, SourcePath: "/good", MatchType: redirectmatch.Exact, Target: "/ok", StatusCode: 302},
	}
	set, errs := redirectmatch.BuildRuleSet(specs, p)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	dec := redirectmatch.Resolve(set, "/good", "")
	if !dec.Matched {
		t.Error("good rule should resolve even though another spec was invalid")
	}
}

func TestValidateNoLoop_SelfRedirect(t *testing.T) {
	candidate := redirectmatch.RuleSpec{ID: 1, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/foo", StatusCode: 301}
	err := redirectmatch.ValidateNoLoop(candidate, nil)
	if err == nil {
		t.Error("ValidateNoLoop should reject self-redirect")
	}
}

func TestValidateNoLoop_DirectCycle(t *testing.T) {
	// candidate: A -> B; existing has B -> A (direct 2-cycle)
	candidate := redirectmatch.RuleSpec{ID: 2, SourcePath: "/a", MatchType: redirectmatch.Exact, Target: "/b", StatusCode: 301}
	existing := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: "/b", MatchType: redirectmatch.Exact, Target: "/a", StatusCode: 301},
	}
	err := redirectmatch.ValidateNoLoop(candidate, existing)
	if err == nil {
		t.Error("ValidateNoLoop should reject direct 2-cycle A->B when B->A exists")
	}
}

func TestValidateNoLoop_AllowsNoCycle(t *testing.T) {
	// candidate: A -> B; existing has B -> C (no cycle, just chain)
	candidate := redirectmatch.RuleSpec{ID: 2, SourcePath: "/a", MatchType: redirectmatch.Exact, Target: "/b", StatusCode: 301}
	existing := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: "/b", MatchType: redirectmatch.Exact, Target: "/c", StatusCode: 301},
	}
	err := redirectmatch.ValidateNoLoop(candidate, existing)
	if err != nil {
		t.Errorf("ValidateNoLoop should allow A->B when B->C (no cycle): %v", err)
	}
}

func TestValidateNoLoop_AllowsUnrelated(t *testing.T) {
	candidate := redirectmatch.RuleSpec{ID: 1, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/bar", StatusCode: 301}
	existing := []redirectmatch.RuleSpec{
		{ID: 2, SourcePath: "/baz", MatchType: redirectmatch.Exact, Target: "/qux", StatusCode: 301},
	}
	err := redirectmatch.ValidateNoLoop(candidate, existing)
	if err != nil {
		t.Errorf("ValidateNoLoop should allow unrelated rules: %v", err)
	}
}
