package redirectmatch_test

import (
	"net/url"
	"testing"

	"github.com/anatolykoptev/go-kit/redirectmatch"
)

// A regex rule whose target starts with "/$1" must never expand into a
// Location that leaves the origin. The matched path is percent-decoded, so
// "%2F" and "%5C" arrive in the capture as "/" and "\\".
//
// Falsification: delete the isSameOriginPath check in buildDecision
// (resolve.go) and every off-site case goes RED.
func TestResolve_CaptureCannotLeaveTheOrigin(t *testing.T) {
	set := mustBuild(t, []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: `^/old/(.+)$`, MatchType: redirectmatch.Regex, Target: "/$1", StatusCode: 301},
	}, redirectmatch.DefaultPolicy())

	for _, path := range []string{
		"/old/%2Fevil.example",
		"/old/%5Cevil.example",
		"/old/%2F%2Fevil.example",
		"/old/%09%5Cevil.example",
	} {
		if dec := redirectmatch.Resolve(set, path, ""); dec.Matched {
			t.Errorf("%s: redirected to %q, want no match", path, dec.Location)
		}
	}

	// Ordinary paths keep working, spaces included (a space is a path byte).
	for path, want := range map[string]string{"/old/news/item": "/news/item", "/old/a%20b": "/a b"} {
		if dec := redirectmatch.Resolve(set, path, ""); !dec.Matched || dec.Location != want {
			t.Errorf("%s: got matched=%v location=%q, want %s", path, dec.Matched, dec.Location, want)
		}
	}
}

// A capture must never choose the redirect's host: a reference inside the
// authority of an absolute or protocol-relative target, or a target that does
// not start with "/", is a compile error rather than a rule that
// "/blog@evil.example" can steer off-site.
//
// Falsification: return nil from targetCaptureError (compile.go) and this goes RED.
func TestCompile_CaptureCannotChooseTheHost(t *testing.T) {
	for _, target := range []string{
		"https://blog.example.org$1",
		"https://$1",
		"HTTPS://$1/x",
		"//$1",
		"//cdn.example.org$1/x",
		"https://user$1@blog.example.org/",
		"$1",
		"news/$1",
		"javascript:$1",
	} {
		spec := redirectmatch.RuleSpec{ID: 1, SourcePath: `^/blog(.*)$`, MatchType: redirectmatch.Regex, Target: target, StatusCode: 301}
		if _, err := redirectmatch.Compile(spec); err == nil {
			t.Errorf("Target %q compiled, want an error", target)
		}
	}
	for _, target := range []string{
		"/$1",
		"https://blog.example.org/$1",
		"https://blog.example.org?from=$1",
		"//cdn.example.org/$1",
	} {
		spec := redirectmatch.RuleSpec{ID: 1, SourcePath: `^/blog(.*)$`, MatchType: redirectmatch.Regex, Target: target, StatusCode: 301}
		if _, err := redirectmatch.Compile(spec); err != nil {
			t.Errorf("Target %q: %v, want it to compile", target, err)
		}
	}
}

// With the host fixed by the template, no request path moves it.
func TestResolve_FixedHostStaysFixed(t *testing.T) {
	set := mustBuild(t, []redirectmatch.RuleSpec{
		{ID: 1, SourcePath: `^/blog/(.*)$`, MatchType: redirectmatch.Regex, Target: "https://blog.example.org/$1", StatusCode: 302},
		{ID: 2, SourcePath: `^/cdn/(.*)$`, MatchType: redirectmatch.Regex, Target: "//cdn.example.org/$1", StatusCode: 302},
	}, redirectmatch.DefaultPolicy())

	for _, path := range []string{
		"/blog/post", "/blog/@evil.example", "/blog/%2F%2Fevil.example", "/blog/%5Cevil.example",
		"/cdn/a.js", "/cdn/@evil.example", "/cdn/%2F%2Fevil.example",
	} {
		dec := redirectmatch.Resolve(set, path, "")
		if !dec.Matched {
			t.Errorf("%s: no match, want a redirect", path)
			continue
		}
		u, err := url.Parse(dec.Location)
		if err != nil {
			t.Errorf("%s: Location %q does not parse: %v", path, dec.Location, err)
			continue
		}
		if u.Host != "blog.example.org" && u.Host != "cdn.example.org" {
			t.Errorf("%s: Location %q has host %q", path, dec.Location, u.Host)
		}
	}
}
