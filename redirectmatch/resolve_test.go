package redirectmatch_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/redirectmatch"
)

func mustBuild(t *testing.T, specs []redirectmatch.RuleSpec, p redirectmatch.Policy) *redirectmatch.RuleSet {
	t.Helper()
	set, errs := redirectmatch.BuildRuleSet(specs, p)
	if len(errs) > 0 {
		t.Fatalf("BuildRuleSet: unexpected errors: %v", errs)
	}
	return set
}

func TestResolve_ExactMatch(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/bar", StatusCode: 301},
	}
	set := mustBuild(t, specs, p)

	dec := redirectmatch.Resolve(set, "/foo", "")
	if !dec.Matched {
		t.Fatal("expected match")
	}
	if dec.StatusCode != 301 {
		t.Errorf("StatusCode = %d, want 301", dec.StatusCode)
	}
	if dec.Location != "/bar" {
		t.Errorf("Location = %q, want /bar", dec.Location)
	}
}

func TestResolve_ExactBeatsByRegex(t *testing.T) {
	// Exact rule at lower priority than regex, but exact should still win for exact tier
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: `^/foo$`, MatchType: redirectmatch.Regex, Target: "/from-regex", StatusCode: 301, Priority: 0},
		{ID: 2, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/from-exact", StatusCode: 302, Priority: 10},
	}
	set := mustBuild(t, specs, p)

	dec := redirectmatch.Resolve(set, "/foo", "")
	if !dec.Matched {
		t.Fatal("expected match")
	}
	if dec.Location != "/from-exact" {
		t.Errorf("Location = %q, want /from-exact (exact tier beats ordered tier)", dec.Location)
	}
}

func TestResolve_OrderedTierPriority(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	// Two regex rules: lower Priority int = tried first
	specs := []redirectmatch.RuleSpec{
		{ID: 2, SourcePath: `^/foo$`, MatchType: redirectmatch.Regex, Target: "/second", StatusCode: 301, Priority: 10},
		{ID: 1, SourcePath: `^/foo$`, MatchType: redirectmatch.Regex, Target: "/first", StatusCode: 302, Priority: 5},
	}
	set := mustBuild(t, specs, p)

	dec := redirectmatch.Resolve(set, "/foo", "")
	if !dec.Matched {
		t.Fatal("expected match")
	}
	if dec.Location != "/first" {
		t.Errorf("Location = %q, want /first (lower priority int wins)", dec.Location)
	}
}

func TestResolve_OrderedTierTiebreakerByID(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	// Same priority, lower ID wins
	specs := []redirectmatch.RuleSpec{
		{ID: 5, SourcePath: `^/foo$`, MatchType: redirectmatch.Regex, Target: "/id5", StatusCode: 301, Priority: 0},
		{ID: 3, SourcePath: `^/foo$`, MatchType: redirectmatch.Regex, Target: "/id3", StatusCode: 301, Priority: 0},
	}
	set := mustBuild(t, specs, p)

	dec := redirectmatch.Resolve(set, "/foo", "")
	if !dec.Matched {
		t.Fatal("expected match")
	}
	if dec.Location != "/id3" {
		t.Errorf("Location = %q, want /id3 (lower ID wins on tie)", dec.Location)
	}
}

func TestResolve_PrefixMatch(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: "/blog", MatchType: redirectmatch.Prefix, Target: "/articles", StatusCode: 301, Priority: 0},
	}
	set := mustBuild(t, specs, p)

	dec := redirectmatch.Resolve(set, "/blog/post", "")
	if !dec.Matched {
		t.Fatal("expected prefix match for /blog/post")
	}
	if dec.StatusCode != 301 {
		t.Errorf("StatusCode = %d, want 301", dec.StatusCode)
	}
}

func TestResolve_RegexExpansion1(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: `^/old/(\d+)$`, MatchType: redirectmatch.Regex, Target: "/new/$1", StatusCode: 301, Priority: 0},
	}
	set := mustBuild(t, specs, p)

	dec := redirectmatch.Resolve(set, "/old/42", "")
	if !dec.Matched {
		t.Fatal("expected regex match")
	}
	if dec.Location != "/new/42" {
		t.Errorf("Location = %q, want /new/42", dec.Location)
	}
}

func TestResolve_RegexExpansion2(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: `^/(\w+)/(\d+)$`, MatchType: redirectmatch.Regex, Target: "/$2/$1", StatusCode: 301, Priority: 0},
	}
	set := mustBuild(t, specs, p)

	dec := redirectmatch.Resolve(set, "/posts/99", "")
	if !dec.Matched {
		t.Fatal("expected regex match")
	}
	if dec.Location != "/99/posts" {
		t.Errorf("Location = %q, want /99/posts", dec.Location)
	}
}

func TestResolve_StatusCodes(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	for _, code := range []int{301, 302, 307, 308} {
		spec := redirectmatch.RuleSpec{ID: int64(code), SourcePath: "/src", MatchType: redirectmatch.Exact, Target: "/dst", StatusCode: code}
		set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)
		dec := redirectmatch.Resolve(set, "/src", "")
		if !dec.Matched {
			t.Errorf("code %d: expected match", code)
			continue
		}
		if dec.StatusCode != code {
			t.Errorf("code %d: StatusCode = %d, want %d", code, dec.StatusCode, code)
		}
		if dec.Location != "/dst" {
			t.Errorf("code %d: Location = %q, want /dst", code, dec.Location)
		}
	}
}

func TestResolve_410Gone(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/deleted", MatchType: redirectmatch.Exact, Target: "", StatusCode: 410}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/deleted", "")
	if !dec.Matched {
		t.Fatal("expected match for 410")
	}
	if dec.StatusCode != 410 {
		t.Errorf("StatusCode = %d, want 410", dec.StatusCode)
	}
	if dec.Location != "" {
		t.Errorf("Location = %q, want empty for 410", dec.Location)
	}
}

func TestResolve_451Legal(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/legal", MatchType: redirectmatch.Exact, Target: "", StatusCode: 451}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/legal", "")
	if !dec.Matched {
		t.Fatal("expected match for 451")
	}
	if dec.StatusCode != 451 {
		t.Errorf("StatusCode = %d, want 451", dec.StatusCode)
	}
	if dec.Location != "" {
		t.Errorf("Location = %q, want empty for 451", dec.Location)
	}
}

func TestResolve_QueryIgnore(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/x", MatchType: redirectmatch.Exact, Target: "/y", StatusCode: 301, QueryHandling: redirectmatch.QIgnore}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/x", "a=1")
	if !dec.Matched {
		t.Fatal("expected match with QIgnore")
	}
	if dec.Location != "/y" {
		t.Errorf("Location = %q, want /y (no query appended with QIgnore)", dec.Location)
	}
}

func TestResolve_QueryPass(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/x", MatchType: redirectmatch.Exact, Target: "/y", StatusCode: 301, QueryHandling: redirectmatch.QPass}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/x", "a=1")
	if !dec.Matched {
		t.Fatal("expected match with QPass")
	}
	if dec.Location != "/y?a=1" {
		t.Errorf("Location = %q, want /y?a=1", dec.Location)
	}
}

func TestResolve_QueryPassWithExistingQuery(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/x", MatchType: redirectmatch.Exact, Target: "/y?z=9", StatusCode: 301, QueryHandling: redirectmatch.QPass}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/x", "a=1")
	if !dec.Matched {
		t.Fatal("expected match")
	}
	if dec.Location != "/y?z=9&a=1" {
		t.Errorf("Location = %q, want /y?z=9&a=1", dec.Location)
	}
}

func TestResolve_QueryExact_Match(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	// QExact rule: SourcePath includes query embedded as "/?p=123"
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/?p=123", MatchType: redirectmatch.Exact, Target: "/post/123", StatusCode: 301, QueryHandling: redirectmatch.QExact}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/", "p=123")
	if !dec.Matched {
		t.Fatal("expected QExact match with path=/ query=p=123")
	}
	if dec.Location != "/post/123" {
		t.Errorf("Location = %q, want /post/123", dec.Location)
	}
}

func TestResolve_QueryExact_NoMatchWrongQuery(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/?p=123", MatchType: redirectmatch.Exact, Target: "/post/123", StatusCode: 301, QueryHandling: redirectmatch.QExact}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/", "p=999")
	if dec.Matched {
		t.Error("should NOT match QExact rule with different query (p=999)")
	}
}

func TestResolve_QueryExact_NoMatchEmptyQuery(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/?p=123", MatchType: redirectmatch.Exact, Target: "/post/123", StatusCode: 301, QueryHandling: redirectmatch.QExact}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/", "")
	if dec.Matched {
		t.Error("should NOT match QExact rule with empty query")
	}
}

func TestResolve_Normalization(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	// Rule stored as /x (normalized), incoming path is /X/ (uppercase + trailing slash)
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/x", MatchType: redirectmatch.Exact, Target: "/y", StatusCode: 301}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/X/", "")
	if !dec.Matched {
		t.Fatal("expected match after normalization of /X/ -> /x")
	}
}

func TestResolve_PercentDecode(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	// Rule stored as /foo bar (decoded), incoming path is /foo%20bar
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/foo bar", MatchType: redirectmatch.Exact, Target: "/ok", StatusCode: 301}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/foo%20bar", "")
	if !dec.Matched {
		t.Fatal("expected match after percent-decode of /foo%20bar -> /foo bar")
	}
}

func TestResolve_SingleHop(t *testing.T) {
	// A -> B and B -> C: resolving A should return B (not chase to C)
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: "/a", MatchType: redirectmatch.Exact, Target: "/b", StatusCode: 301},
		{ID: 2, SourcePath: "/b", MatchType: redirectmatch.Exact, Target: "/c", StatusCode: 301},
	}
	set := mustBuild(t, specs, p)

	dec := redirectmatch.Resolve(set, "/a", "")
	if !dec.Matched {
		t.Fatal("expected match for /a")
	}
	if dec.Location != "/b" {
		t.Errorf("Location = %q, want /b (single-hop, not chased to /c)", dec.Location)
	}
}

func TestResolve_Miss(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	spec := redirectmatch.RuleSpec{ID: 1, SourcePath: "/foo", MatchType: redirectmatch.Exact, Target: "/bar", StatusCode: 301}
	set := mustBuild(t, []redirectmatch.RuleSpec{spec}, p)

	dec := redirectmatch.Resolve(set, "/unknown", "")
	if dec.Matched {
		t.Error("expected no match for unknown path")
	}
}
