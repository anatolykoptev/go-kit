package redirectmatch_test

import (
	"testing"

	"github.com/anatolykoptev/go-kit/redirectmatch"
)

// A regex rule whose relative target starts with a capture must never expand
// into a Location that leaves the origin. The matched path is percent-decoded,
// so "%2F" and "%5C" arrive in the capture as "/" and "\".
//
// Falsification: delete the isSameOriginPath check in buildDecision
// (resolve.go) and every off-site case goes RED.
func TestResolve_CaptureCannotLeaveTheOrigin(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: `^/old/(.+)$`, MatchType: redirectmatch.Regex, Target: "/$1", StatusCode: 301},
	}
	set := mustBuild(t, specs, p)

	for _, path := range []string{
		"/old/%2Fevil.example",
		"/old/%5Cevil.example",
		"/old/%2F%2Fevil.example",
		"/old/%09%5Cevil.example",
	} {
		dec := redirectmatch.Resolve(set, path, "")
		if dec.Matched {
			t.Errorf("%s: redirected to %q, want no match", path, dec.Location)
		}
	}

	// The same rule keeps working for ordinary paths.
	dec := redirectmatch.Resolve(set, "/old/news/item", "")
	if !dec.Matched || dec.Location != "/news/item" {
		t.Errorf("/old/news/item: got matched=%v location=%q, want /news/item", dec.Matched, dec.Location)
	}
}

// A relative template that does not start with "/" is guarded too: "$1" fed
// "https://evil.example" (via "%2F%2F") must not become an absolute Location.
//
// Falsification: drop the `loc[0] != '/'` test in isSameOriginPath
// (resolve.go) and this goes RED.
func TestResolve_BareCaptureCannotBecomeAbsolute(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: `^/go(.*)$`, MatchType: redirectmatch.Regex, Target: "$1", StatusCode: 302},
	}
	set := mustBuild(t, specs, p)
	for _, path := range []string{"/gohttps:%2F%2Fevil.example", "/go%2F%2Fevil.example"} {
		if dec := redirectmatch.Resolve(set, path, ""); dec.Matched {
			t.Errorf("%s: redirected to %q, want no match", path, dec.Location)
		}
	}
	// A space is an ordinary path byte, not a reason to refuse.
	if dec := redirectmatch.Resolve(set, "/go/a%20b", ""); !dec.Matched || dec.Location != "/a b" {
		t.Errorf("/go/a%%20b: got matched=%v location=%q, want /a b", dec.Matched, dec.Location)
	}
}

// An absolute template is an operator's chosen destination; captures inside
// its path are not second-guessed.
func TestResolve_AbsoluteTargetUnaffected(t *testing.T) {
	p := redirectmatch.DefaultPolicy()
	specs := []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: `^/go/(.+)$`, MatchType: redirectmatch.Regex, Target: "https://example.org/$1", StatusCode: 302},
	}
	set := mustBuild(t, specs, p)
	dec := redirectmatch.Resolve(set, "/go/page", "")
	if !dec.Matched || dec.Location != "https://example.org/page" {
		t.Errorf("got matched=%v location=%q, want https://example.org/page", dec.Matched, dec.Location)
	}
}
