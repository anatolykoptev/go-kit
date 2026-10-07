package svcauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// recorder serves one origin and records the header each request carried.
func recorder(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get(HeaderInternalSecret))
		if to := r.URL.Query().Get("redirect"); to != "" {
			http.Redirect(w, r, to, http.StatusFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestTransport_SendsSecretOnlyToRoutedOrigin(t *testing.T) {
	internal, gotInternal := recorder(t)
	external, gotExternal := recorder(t)

	c, err := WrapClient(nil, Route{BaseURL: internal.URL, Secret: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{internal.URL + "/fetch", external.URL + "/page"} {
		resp, err := c.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if len(*gotInternal) != 1 || (*gotInternal)[0] != "s3cret" {
		t.Errorf("internal got %q, want [s3cret]", *gotInternal)
	}
	if len(*gotExternal) != 1 || (*gotExternal)[0] != "" {
		t.Errorf("external got %q, want no header", *gotExternal)
	}
}

// A redirect from the internal service to another origin must not carry
// the secret: libraries wrap clients that also follow public redirects.
func TestTransport_RedirectToOtherOriginDropsSecret(t *testing.T) {
	internal, gotInternal := recorder(t)
	external, gotExternal := recorder(t)

	c, err := WrapClient(nil, Route{BaseURL: internal.URL, Secret: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(internal.URL + "/?redirect=" + external.URL + "/landing")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(*gotInternal) != 1 || (*gotInternal)[0] != "s3cret" {
		t.Errorf("internal got %q, want [s3cret]", *gotInternal)
	}
	if len(*gotExternal) != 1 || (*gotExternal)[0] != "" {
		t.Errorf("redirect target got %q, want no header", *gotExternal)
	}
}

func TestTransport_DoesNotMutateCallerRequest(t *testing.T) {
	internal, _ := recorder(t)
	c, err := WrapClient(nil, Route{BaseURL: internal.URL, Secret: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, internal.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if req.Header.Get(HeaderInternalSecret) != "" {
		t.Fatal("caller's request was mutated")
	}
}

func TestNew_SkipsEmptyRoutesAndRejectsBadURLs(t *testing.T) {
	tr, err := New(nil, Route{BaseURL: "", Secret: "x"}, Route{BaseURL: "http://a:1", Secret: ""})
	if err != nil {
		t.Fatalf("empty routes should be skipped: %v", err)
	}
	if len(tr.routes) != 0 {
		t.Errorf("routes = %v, want none", tr.routes)
	}
	for _, bad := range []string{"ox-browser:8901", "ftp://h", "://x"} {
		if _, err := New(nil, Route{BaseURL: bad, Secret: "x"}); err == nil || !strings.Contains(err.Error(), "invalid base URL") {
			t.Errorf("%q: err = %v, want invalid base URL", bad, err)
		}
	}
}

func TestOriginKey_DefaultPortsAndCase(t *testing.T) {
	tr, err := New(nil, Route{BaseURL: "http://Ox-Browser", Secret: "x"})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"http://ox-browser:80/read", "http://OX-BROWSER/fetch"} {
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		if _, ok := tr.routes[originKey(req.URL)]; !ok {
			t.Errorf("%s did not match the route", u)
		}
	}
	for _, u := range []string{"https://ox-browser/read", "http://ox-browser:8901/read", "http://ox-browser.evil/read"} {
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		if _, ok := tr.routes[originKey(req.URL)]; ok {
			t.Errorf("%s matched the route; want no match", u)
		}
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv(EnvInternalSecret, "env-secret")
	internal, got := recorder(t)
	tr, err := FromEnv(nil, internal.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Transport: tr}).Get(internal.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(*got) != 1 || (*got)[0] != "env-secret" {
		t.Errorf("got %q, want [env-secret]", *got)
	}
}

// A public server must not be able to redirect the client into a routed
// origin and have the secret attached there. RED-on-revert: drop the
// req.Response walk in secretFor.
func TestTransport_RedirectFromPublicIntoRoutedDropsSecret(t *testing.T) {
	internal, gotInternal := recorder(t)
	public, _ := recorder(t)

	c, err := WrapClient(nil, Route{BaseURL: internal.URL, Secret: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(public.URL + "/?redirect=" + internal.URL + "/api/v1/chrome/tabs")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(*gotInternal) != 1 || (*gotInternal)[0] != "" {
		t.Errorf("internal got %q via a public redirect, want no header", *gotInternal)
	}
}

func TestTransport_RedirectBetweenRoutedOriginsKeepsSecret(t *testing.T) {
	a, gotA := recorder(t)
	b, gotB := recorder(t)
	c, err := WrapClient(nil, Route{BaseURL: a.URL, Secret: "s3cret"}, Route{BaseURL: b.URL, Secret: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(a.URL + "/?redirect=" + b.URL + "/next")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if (*gotA)[0] != "s3cret" || len(*gotB) != 1 || (*gotB)[0] != "s3cret" {
		t.Errorf("a=%q b=%q, want the secret on both routed hops", *gotA, *gotB)
	}
}

func TestTransport_StripsCallerSetSecretOnUnroutedOrigin(t *testing.T) {
	internal, _ := recorder(t)
	external, gotExternal := recorder(t)
	c, err := WrapClient(nil, Route{BaseURL: internal.URL, Secret: "s3cret"})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, external.URL, nil)
	req.Header.Set(HeaderInternalSecret, "manual")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if (*gotExternal)[0] != "" {
		t.Errorf("external got %q, want the caller-set header stripped", *gotExternal)
	}
	if req.Header.Get(HeaderInternalSecret) != "manual" {
		t.Error("caller's request was mutated")
	}
}

func TestNew_RejectsConflictingDuplicateRoutes(t *testing.T) {
	_, err := New(nil, Route{BaseURL: "http://a:1", Secret: "x"}, Route{BaseURL: "http://A:1/", Secret: "y"})
	if err == nil || !strings.Contains(err.Error(), "listed twice") {
		t.Fatalf("err = %v, want duplicate-origin error", err)
	}
	if _, err := New(nil, Route{BaseURL: "http://a:1", Secret: "x"}, Route{BaseURL: "http://a:1", Secret: "x"}); err != nil {
		t.Fatalf("identical duplicate should be accepted: %v", err)
	}
}

func TestOriginKey_TrailingDot(t *testing.T) {
	tr, err := New(nil, Route{BaseURL: "http://ox-browser.:8901", Secret: "x"})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, "http://ox-browser:8901/read", nil)
	if _, ok := tr.secretFor(req); !ok {
		t.Error("trailing-dot route did not match the dotless host")
	}
}
