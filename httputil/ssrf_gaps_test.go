package httputil

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// fakeResolver answers LookupIPAddr from a script. answers[i] is returned for
// the i-th call; the last entry repeats.
type fakeResolver struct {
	mu      sync.Mutex
	answers []fakeAnswer
	calls   int
}

type fakeAnswer struct {
	ips []string
	err error
}

func (f *fakeResolver) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := min(f.calls, len(f.answers)-1)
	f.calls++
	a := f.answers[i]
	if a.err != nil {
		return nil, a.err
	}
	out := make([]net.IPAddr, 0, len(a.ips))
	for _, s := range a.ips {
		out = append(out, net.IPAddr{IP: net.ParseIP(s)})
	}
	return out, nil
}

func (f *fakeResolver) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestIsBlockedIP_SpecialPurposeRanges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ip      string
		blocked bool
	}{
		// 0.0.0.0/8 (issue #289 item 1).
		{"0.0.0.0", true}, {"0.1.2.3", true}, {"0.255.255.255", true}, {"1.0.0.0", false}, // repo-standards: ignore SSRF boundary case, the literal must sit exactly at the range edge
		// 192.0.0.0/24.
		{"192.0.0.0", true}, {"192.0.0.9", true}, {"192.0.0.255", true}, {"192.0.1.0", false}, // repo-standards: ignore SSRF boundary case, the literal must sit exactly at the range edge
		// 198.18.0.0/15.
		{"198.17.255.255", false}, {"198.18.0.0", true}, {"198.19.255.255", true}, {"198.20.0.0", false}, // repo-standards: ignore SSRF boundary case, the literal must sit exactly at the range edge
		// 240.0.0.0/4 incl. broadcast.
		{"239.255.255.255", true}, // multicast, already blocked
		{"240.0.0.0", true}, {"255.255.255.254", true}, {"255.255.255.255", true},
		// Documentation + 6to4 relay anycast (beyond the issue).
		{"192.0.2.1", true}, {"198.51.100.1", true}, {"203.0.113.1", true}, {"192.88.99.1", true},
		{"192.0.3.1", false}, {"198.51.101.1", false}, {"203.0.114.1", false}, // repo-standards: ignore SSRF boundary case, the literal must sit exactly at the range edge

		// IPv4-mapped IPv6 is judged by the IPv4 it carries.
		{"::ffff:0.1.2.3", true}, {"::ffff:240.0.0.1", true}, {"::ffff:198.18.0.1", true},
		{"::ffff:192.0.0.1", true}, {"::ffff:8.8.8.8", false},

		// Embedded-IPv4 transition prefixes (carrying a PUBLIC v4 still blocked).
		{"64:ff9b::808:808", true},
		{"64:ff9b:1::1", true},
		{"::ffff:0:808:808", true},                     // SIIT, ::ffff:0:8.8.8.8; repo-standards: ignore SSRF boundary case, the literal must sit exactly at the range edge
		{"::808:808", true},                            // IPv4-compatible
		{"2002:808:808::1", true},                      // 6to4
		{"2001:0:4136:e378:8000:63bf:3fff:fdd2", true}, // Teredo
		{"2001:1::1", true},                            // inside 2001::/23; repo-standards: ignore SSRF boundary case, the literal must sit exactly at the range edge
		{"2001:200::1", false},                         // just outside 2001::/23; repo-standards: ignore SSRF boundary case, the literal must sit exactly at the range edge

		// Other IPv6 special-purpose.
		{"100::1", true}, {"100:0:0:1::1", false}, // repo-standards: ignore SSRF boundary case, the literal must sit exactly at the range edge
		{"2001:db8::1", true}, {"3fff::1", true}, {"3fff:fff::1", true}, {"3fff:1000::1", false}, // repo-standards: ignore SSRF boundary case, the literal must sit exactly at the range edge
		{"5f00::1", true}, {"fec0::1", true}, {"fe80::1", true}, {"fc00::1", true}, // repo-standards: ignore SSRF boundary case, the literal must sit exactly at the range edge
		{"ff02::1", true}, {"ff01::1", true}, {"::1", true}, {"::", true},

		// Public controls.
		{"8.8.8.8", false}, {"1.1.1.1", false}, {"9.9.9.9", false},
		{"2606:4700:4700::1111", false}, {"2001:4860:4860::8888", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("test setup: %q does not parse", tt.ip)
			}
			if got := IsBlockedIP(ip); got != tt.blocked {
				t.Errorf("IsBlockedIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
			}
		})
	}
}

func TestIsBlockedIP_MalformedFailsClosed(t *testing.T) {
	t.Parallel()
	for _, ip := range []net.IP{{}, {1, 2, 3}, make(net.IP, 5)} {
		if !IsBlockedIP(ip) {
			t.Errorf("IsBlockedIP(%v) = false, want true (malformed fails closed)", []byte(ip))
		}
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("test setup: %v", err)
	}
	return u
}

func TestCheckURL_ResolverFailures(t *testing.T) {
	t.Parallel()
	errDNS := errors.New("dns: server misbehaving")
	tests := []struct {
		name    string
		answer  fakeAnswer
		blocked bool
	}{
		{"dns error", fakeAnswer{err: errDNS}, true},
		{"zero addresses", fakeAnswer{ips: nil}, true},
		{"all public", fakeAnswer{ips: []string{"9.9.9.9", "2606:4700:4700::1111"}}, false},
		{"mixed public and internal", fakeAnswer{ips: []string{"9.9.9.9", "10.0.0.1"}}, true},
		{"mixed internal last, new range", fakeAnswer{ips: []string{"9.9.9.9", "0.1.2.3"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := checkURL(context.Background(), mustURL(t, "https://example.com/"), &fakeResolver{answers: []fakeAnswer{tt.answer}})
			if got := errors.Is(err, ErrSSRFBlocked); got != tt.blocked {
				t.Fatalf("errors.Is(err, ErrSSRFBlocked) = %v, want %v (err=%v)", got, tt.blocked, err)
			}
			if tt.answer.err != nil && !errors.Is(err, errDNS) {
				t.Errorf("original resolver error lost from chain: %v", err)
			}
		})
	}
}

func TestCheckURL_PublicResolver_DefaultsToNetResolver(t *testing.T) {
	t.Parallel()
	// Literal IP path never touches DNS; proves the exported entry point still works.
	if err := CheckURL(context.Background(), mustURL(t, "http://0.1.2.3/")); !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("CheckURL(0.1.2.3) = %v, want ErrSSRFBlocked", err)
	}
}

func TestCheckURL_AltEncodedHosts(t *testing.T) {
	t.Parallel()
	refused := []string{
		// decimal / hex / octal single-part
		"2130706433", "0x7f000001", "0X7F000001", "017700000001", "4294967295",
		// dotted mixes
		"0x7f.0.0.1", "0x7f.0x0.0x0.0x1", "0177.0.0.1", "012.0.0.1", "0177.0.0.01", "127.0.0.01",
		// short forms (a.b, a.b.c)
		"127.1", "127.0.1", "10.1", "0x7f.1", "0177.1", "0.0", "0", "0x", "0x.0.0.1",
		// invalid octal digit still numeric-looking: refused conservatively
		"08.0.0.1",
		// trailing dot (absolute name)
		"127.1.", "2130706433.",
		// full-width digits fold to ASCII under IDNA
		"１２７．０．０．１", "２１３０７０６４３３",
	}
	allowed := []string{
		"0x.org", "docs.0x.org", "0x7f.example.com", "01.example.com", "example.com",
		"a.01.example.com", "example.01", "1.2.3.4.5", "0xg.org", "www.007.agency",
		"127.0.0.1.example.com", "localhost.0x7f000001.example.com",
	}
	public := []fakeAnswer{{ips: []string{"9.9.9.9"}}}
	for _, h := range refused {
		t.Run("refused/"+h, func(t *testing.T) {
			t.Parallel()
			res := &fakeResolver{answers: public}
			u := &url.URL{Scheme: "http", Host: h}
			err := checkURL(context.Background(), u, res)
			if !errors.Is(err, ErrSSRFBlocked) || !strings.Contains(err.Error(), "non-standard IP encoding") {
				t.Errorf("host %q: err = %v, want ErrSSRFBlocked (alt encoding)", h, err)
			}
			if res.callCount() != 0 {
				t.Errorf("host %q: resolver called %d times, want 0 (must be refused before DNS)", h, res.callCount())
			}
		})
	}
	for _, h := range allowed {
		t.Run("allowed/"+h, func(t *testing.T) {
			t.Parallel()
			res := &fakeResolver{answers: public}
			u := &url.URL{Scheme: "http", Host: h}
			if err := checkURL(context.Background(), u, res); err != nil {
				t.Errorf("host %q: err = %v, want nil", h, err)
			}
			if res.callCount() != 1 {
				t.Errorf("host %q: resolver called %d times, want 1", h, res.callCount())
			}
		})
	}
}

// dialerThroughClient returns the guarded *http.Transport's DialContext.
func dialerThroughClient(t *testing.T, c *http.Client) func(context.Context, string, string) (net.Conn, error) {
	t.Helper()
	g, ok := c.Transport.(*guardedRoundTripper)
	if !ok {
		t.Fatalf("transport is %T, want *guardedRoundTripper", c.Transport)
	}
	tr, ok := g.next.(*http.Transport)
	if !ok {
		t.Fatalf("inner transport is %T, want *http.Transport", g.next)
	}
	return tr.DialContext
}

func TestNewSSRFGuardedClient_WithDialer_KeepsCallerControl(t *testing.T) {
	t.Parallel()
	errCaller := errors.New("caller control refused")

	t.Run("caller control runs, then guard blocks internal", func(t *testing.T) {
		t.Parallel()
		var seen atomic.Int32
		d := &net.Dialer{Control: func(_, _ string, _ syscall.RawConn) error {
			seen.Add(1)
			return nil
		}}
		c := NewSSRFGuardedClient(&http.Client{Transport: &http.Transport{}}, WithDialer(d))
		_, err := dialerThroughClient(t, c)(context.Background(), "tcp", "127.0.0.1:9")
		if !errors.Is(err, ErrSSRFBlocked) {
			t.Fatalf("dial 127.0.0.1 err = %v, want ErrSSRFBlocked", err)
		}
		if seen.Load() != 1 {
			t.Errorf("caller Control ran %d times, want 1 (hook lost)", seen.Load())
		}
		if d.Control == nil {
			t.Error("caller's net.Dialer was mutated")
		}
	})

	t.Run("caller control can refuse a public address before any connect", func(t *testing.T) {
		t.Parallel()
		d := &net.Dialer{Timeout: 200 * time.Millisecond, Control: func(_, address string, _ syscall.RawConn) error {
			if strings.HasPrefix(address, "8.8.8.8:") {
				return errCaller
			}
			return nil
		}}
		c := NewSSRFGuardedClient(nil, WithDialer(d))
		_, err := dialerThroughClient(t, c)(context.Background(), "tcp", "8.8.8.8:80")
		if !errors.Is(err, errCaller) {
			t.Fatalf("err = %v, want the caller's Control error", err)
		}
	})

	t.Run("no option: internal still blocked", func(t *testing.T) {
		t.Parallel()
		c := NewSSRFGuardedClient(&http.Client{Transport: &http.Transport{}})
		_, err := dialerThroughClient(t, c)(context.Background(), "tcp", "0.1.2.3:80")
		if !errors.Is(err, ErrSSRFBlocked) {
			t.Fatalf("err = %v, want ErrSSRFBlocked", err)
		}
	})
}

// A name that resolves public at the pre-request check and to loopback at
// connect time (DNS rebinding) must be refused and must never reach the server.
func TestNewSSRFGuardedClient_DNSRebindRefusedAtDial(t *testing.T) {
	t.Parallel()
	var hit atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit.Store(true) }))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())

	res := &fakeResolver{answers: []fakeAnswer{
		{ips: []string{"9.9.9.9"}},   // pre-request CheckURL: public
		{ips: []string{"127.0.0.1"}}, // connect time: rebound
	}}
	c := NewSSRFGuardedClient(&http.Client{Transport: &http.Transport{}}, WithResolver(res))
	resp, err := c.Get("http://rebind.test:" + port + "/") //nolint:noctx // test
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("err = %v, want ErrSSRFBlocked", err)
	}
	if hit.Load() {
		t.Error("server was reached despite rebind to loopback")
	}
	if res.callCount() < 2 {
		t.Errorf("resolver called %d times, want >= 2 (pre-check + dial)", res.callCount())
	}
}

// Every resolved address is judged at dial time, not only the first.
func TestNewSSRFGuardedClient_DialRefusesAnyBlockedResolvedAddress(t *testing.T) {
	t.Parallel()
	res := &fakeResolver{answers: []fakeAnswer{{ips: []string{"9.9.9.9", "198.18.0.1"}}}}
	c := NewSSRFGuardedClient(&http.Client{Transport: &http.Transport{}}, WithResolver(res))
	_, err := dialerThroughClient(t, c)(context.Background(), "tcp", "multi.test:80")
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Fatalf("err = %v, want ErrSSRFBlocked", err)
	}
}

func TestNewSSRFGuardedClient_DialDNSErrorIsBlocked(t *testing.T) {
	t.Parallel()
	errDNS := errors.New("no such host")
	res := &fakeResolver{answers: []fakeAnswer{{err: errDNS}}}
	c := NewSSRFGuardedClient(nil, WithResolver(res))
	_, err := dialerThroughClient(t, c)(context.Background(), "tcp", "x.test:80")
	if !errors.Is(err, ErrSSRFBlocked) || !errors.Is(err, errDNS) {
		t.Fatalf("err = %v, want ErrSSRFBlocked wrapping the resolver error", err)
	}
}

// stubRT answers from a function and counts calls.
type stubRT struct {
	calls atomic.Int32
	fn    func(*http.Request) *http.Response
}

func (s *stubRT) RoundTrip(r *http.Request) (*http.Response, error) {
	s.calls.Add(1)
	return s.fn(r), nil
}

// A public first hop that redirects to cloud metadata must not be followed.
func TestNewSSRFGuardedClient_RedirectToBlockedAddressRefused(t *testing.T) {
	t.Parallel()
	for _, loc := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://0.1.2.3/",
		"http://198.18.0.1/",
		"http://2130706433/",
		"http://rebinds-internal.test/",
	} {
		t.Run(loc, func(t *testing.T) {
			t.Parallel()
			rt := &stubRT{fn: func(r *http.Request) *http.Response {
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{loc}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    r,
				}
			}}
			res := &fakeResolver{answers: []fakeAnswer{
				{ips: []string{"9.9.9.9"}},
				{ips: []string{"10.0.0.5"}},
			}}
			c := NewSSRFGuardedClient(&http.Client{Transport: rt}, WithResolver(res))
			resp, err := c.Get("http://public.test/") //nolint:noctx // test
			if resp != nil {
				_ = resp.Body.Close()
			}
			if !errors.Is(err, ErrSSRFBlocked) {
				t.Fatalf("err = %v, want ErrSSRFBlocked", err)
			}
			if n := rt.calls.Load(); n != 1 {
				t.Errorf("underlying transport called %d times, want 1 (redirect hop must not be followed)", n)
			}
		})
	}
}

// Go ignores Dialer.Control when ControlContext is non-nil, so the guard must
// live in ControlContext or a caller's ControlContext dialer skips it.
func TestGuardedDial_ControlContextDialerStillGuarded(t *testing.T) {
	t.Parallel()
	errCaller := errors.New("caller control refused")
	type mk func(d *net.Dialer) func(context.Context, string, string) (net.Conn, error)
	paths := map[string]mk{
		"GuardedDialContext": GuardedDialContext,
		"WithDialer": func(d *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
			return dialerThroughClient(t, NewSSRFGuardedClient(nil, WithDialer(d)))
		},
	}
	for name, build := range paths {
		t.Run(name+"/hook returns nil", func(t *testing.T) {
			t.Parallel()
			var seen atomic.Int32
			d := &net.Dialer{ControlContext: func(context.Context, string, string, syscall.RawConn) error {
				seen.Add(1)
				return nil
			}}
			_, err := build(d)(context.Background(), "tcp", "127.0.0.1:9")
			if !errors.Is(err, ErrSSRFBlocked) {
				t.Fatalf("dial 127.0.0.1 err = %v, want ErrSSRFBlocked (guard skipped)", err)
			}
			if seen.Load() != 1 {
				t.Errorf("caller ControlContext ran %d times, want 1", seen.Load())
			}
		})
		t.Run(name+"/hook returns error", func(t *testing.T) {
			t.Parallel()
			d := &net.Dialer{Timeout: 200 * time.Millisecond, ControlContext: func(context.Context, string, string, syscall.RawConn) error {
				return errCaller
			}}
			_, err := build(d)(context.Background(), "tcp", "8.8.8.8:80")
			if !errors.Is(err, errCaller) {
				t.Fatalf("err = %v, want the caller's ControlContext error", err)
			}
		})
		t.Run(name+"/both set: ControlContext wins, Control not run", func(t *testing.T) {
			t.Parallel()
			var ctrl, ctrlCtx atomic.Int32
			d := &net.Dialer{
				Control:        func(string, string, syscall.RawConn) error { ctrl.Add(1); return nil },
				ControlContext: func(context.Context, string, string, syscall.RawConn) error { ctrlCtx.Add(1); return nil },
			}
			_, err := build(d)(context.Background(), "tcp", "127.0.0.1:9")
			if !errors.Is(err, ErrSSRFBlocked) {
				t.Fatalf("err = %v, want ErrSSRFBlocked", err)
			}
			if ctrlCtx.Load() != 1 || ctrl.Load() != 0 {
				t.Errorf("ControlContext ran %d (want 1), Control ran %d (want 0)", ctrlCtx.Load(), ctrl.Load())
			}
		})
	}
}
