package httputil

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestIsBlockedIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		// Loopback.
		{"loopback v4", "127.0.0.1", true},
		{"loopback v4 range", "127.255.255.254", true},
		{"loopback v6", "::1", true},

		// RFC1918 private.
		{"rfc1918 10/8", "10.0.0.1", true},
		{"rfc1918 172.16/12 low", "172.16.0.1", true},
		{"rfc1918 172.16/12 high", "172.31.255.255", true},
		{"rfc1918 192.168/16", "192.168.1.1", true},
		{"just outside 172.16/12 low", "172.15.255.255", false},
		{"just outside 172.16/12 high", "172.32.0.0", false},

		// RFC4193 unique-local IPv6.
		{"rfc4193 ula", "fc00::1", true},
		{"rfc4193 ula fd", "fd12:3456:789a:1::1", true},

		// Link-local (includes cloud metadata 169.254.169.254).
		{"link-local v4", "169.254.1.1", true},
		{"cloud metadata", "169.254.169.254", true},
		{"link-local v6", "fe80::1", true},

		// Unspecified.
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v6", "::", true},

		// Multicast.
		{"multicast v4", "224.0.0.1", true},
		{"multicast v6", "ff02::1", true},

		// IPv4-mapped-IPv6 of blocked addresses.
		{"ipv4-mapped private", "::ffff:10.0.0.1", true},
		{"ipv4-mapped loopback", "::ffff:127.0.0.1", true},
		{"ipv4-mapped link-local", "::ffff:169.254.169.254", true},

		// CGNAT (RFC 6598) — the superset this package adds over the two
		// pre-existing guards it consolidates.
		{"cgnat low", "100.64.0.1", true},
		{"cgnat high", "100.127.255.254", true},
		{"just outside cgnat low", "100.63.255.255", false},
		{"just outside cgnat high", "100.128.0.0", false},
		{"ipv4-mapped cgnat", "::ffff:100.64.0.1", true},

		// NAT64 well-known prefix (RFC 6052).
		{"nat64", "64:ff9b::192.0.2.1", true},
		{"nat64 low", "64:ff9b::1", true},

		// 6to4 (RFC 3056).
		{"6to4", "2002:c000:0204::1", true},

		// IPv4-compatible IPv6 (deprecated, distinct from IPv4-mapped).
		{"ipv4-compatible", "::7f00:1", true}, // ::127.0.0.1

		// Public — must be allowed.
		{"public v4 google dns", "8.8.8.8", false},
		{"public v4 cloudflare dns", "1.1.1.1", false},
		{"public v4 arbitrary", "93.184.216.34", false},
		{"public v6 cloudflare", "2606:4700:4700::1111", false},
		{"ipv4-mapped public", "::ffff:8.8.8.8", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("test setup: %q did not parse as an IP", tt.ip)
			}
			if got := IsBlockedIP(ip); got != tt.blocked {
				t.Errorf("IsBlockedIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
			}
		})
	}
}

func TestIsBlockedIP_NilFailsClosed(t *testing.T) {
	t.Parallel()
	if !IsBlockedIP(nil) {
		t.Error("IsBlockedIP(nil) = false, want true (fail closed)")
	}
}

func TestCheckURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rawURL  string
		blocked bool
	}{
		{"loopback literal", "http://127.0.0.1:8080/x", true},
		{"loopback hostname", "http://localhost/x", true},
		{"private literal", "http://10.9.0.10:8890/", true},
		{"link-local literal (cloud metadata)", "http://169.254.169.254/latest/meta-data/", true},
		{"docker-compose-range private literal", "http://172.18.0.5:8901/read", true},
		{"cgnat literal", "http://100.64.0.5/", true},
		{"public literal ip https", "https://8.8.8.8/", false},
		{"public literal ip http", "http://8.8.8.8/", false},
		{"malformed url", "http://[::1", true},
		{"empty host", "not-a-url", true},

		// Scheme allowlist.
		{"file scheme", "file:///etc/passwd", true},
		{"gopher scheme", "gopher://127.0.0.1:70/", true},
		{"ftp scheme public host", "ftp://8.8.8.8/", true},

		// Alternate-encoded IP literal bypass attempts.
		{"decimal-encoded loopback", "http://2130706433/", true},
		{"hex-encoded loopback", "http://0x7f000001/", true},
		{"octal-dotted loopback", "http://012.0.0.1/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			u, parseErr := url.Parse(tt.rawURL)
			var err error
			if parseErr != nil {
				err = parseErr
			} else {
				err = CheckURL(ctx, u)
			}
			blocked := err != nil
			if blocked != tt.blocked {
				t.Errorf("CheckURL(%q) blocked = %v (err=%v), want blocked=%v", tt.rawURL, blocked, err, tt.blocked)
			}
		})
	}
}

func TestCheckURL_NilURL(t *testing.T) {
	t.Parallel()
	if err := CheckURL(context.Background(), nil); !errors.Is(err, ErrSSRFBlocked) {
		t.Errorf("CheckURL(nil) = %v, want ErrSSRFBlocked", err)
	}
}

func TestCheckRawURL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := CheckRawURL(ctx, "http://127.0.0.1/x"); !errors.Is(err, ErrSSRFBlocked) {
		t.Errorf("CheckRawURL(loopback) = %v, want ErrSSRFBlocked", err)
	}
	if err := CheckRawURL(ctx, "http://8.8.8.8/x"); err != nil {
		t.Errorf("CheckRawURL(public) = %v, want nil", err)
	}
}

// TestGuardedDialContext_BlocksResolvedAddress drives the DialContext func
// returned by GuardedDialContext with an address exactly as net/http would
// pass it AFTER DNS resolution — this is what proves the guard defeats
// DNS-rebinding: the check fires on the literal resolved address, never on a
// hostname string, so it cannot be fooled by a name that resolves public on
// first lookup and private by connect time.
func TestGuardedDialContext_BlocksResolvedAddress(t *testing.T) {
	t.Parallel()
	dial := GuardedDialContext(&net.Dialer{Timeout: time.Second})

	blockedAddrs := []string{
		"127.0.0.1:80",
		"169.254.169.254:80", // cloud metadata
		"10.0.0.1:443",
		"172.18.0.5:8901", // docker-compose bridge range
		"100.64.0.1:80",   // CGNAT
		"[::1]:80",
		"[fe80::1]:80",
		"[64:ff9b::192.0.2.1]:80", // NAT64
	}
	for _, addr := range blockedAddrs {
		addr := addr
		t.Run(addr, func(t *testing.T) {
			t.Parallel()
			_, err := dial(context.Background(), "tcp", addr)
			if err == nil {
				t.Fatalf("dial(%q) succeeded, want ErrSSRFBlocked", addr)
			}
			if !errors.Is(err, ErrSSRFBlocked) {
				t.Errorf("dial(%q) error %v does not wrap ErrSSRFBlocked", addr, err)
			}
		})
	}
}

// TestGuardedDialContext_NilBaseUsesDefaults proves base == nil doesn't
// panic and still guards.
func TestGuardedDialContext_NilBaseUsesDefaults(t *testing.T) {
	t.Parallel()
	dial := GuardedDialContext(nil)
	_, err := dial(context.Background(), "tcp", "127.0.0.1:80")
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Errorf("dial with nil base = %v, want ErrSSRFBlocked", err)
	}
}

// TestDenyBlockedAddress_AllowsPublicAddress proves the exact Control-hook
// body used inside GuardedDialContext permits a public address to proceed —
// the "public allowed" side of the connect-time guard, tested without a real
// network dial (a live outbound connection to 8.8.8.8 would be
// non-deterministic in a sandboxed/offline CI environment).
func TestDenyBlockedAddress_AllowsPublicAddress(t *testing.T) {
	t.Parallel()
	publicAddrs := []string{"8.8.8.8:443", "1.1.1.1:80", "[2606:4700:4700::1111]:443"}
	for _, addr := range publicAddrs {
		addr := addr
		t.Run(addr, func(t *testing.T) {
			t.Parallel()
			if err := denyBlockedAddress("tcp", addr); err != nil {
				t.Errorf("denyBlockedAddress(%q) = %v, want nil (public address must be allowed)", addr, err)
			}
		})
	}
}

// TestNewSSRFGuardedClient_RefusesLoopbackServer is the client-level
// (not just predicate-level) regression test: a real httptest server bound
// to loopback must be refused by the guarded client's connect-time dial.
func TestNewSSRFGuardedClient_RefusesLoopbackServer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("should never be reached"))
	}))
	defer srv.Close()

	client := NewSSRFGuardedClient(nil)
	client.Timeout = 2 * time.Second
	resp, err := client.Get(srv.URL) //nolint:noctx // test: URL is a fixed local httptest server
	if resp != nil {
		resp.Body.Close() //nolint:errcheck
	}
	if err == nil {
		t.Fatal("expected NewSSRFGuardedClient(nil) to refuse the loopback target")
	}
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Errorf("expected error to wrap ErrSSRFBlocked, got: %v", err)
	}
}

// TestNewSSRFGuardedClient_TransportTierPreservesConfig proves the
// *http.Transport tier is a real Clone() — a config marker set on the base
// Transport survives (by value; http.Transport.Clone() deep-copies
// TLSClientConfig via tls.Config.Clone(), so pointer identity is NOT
// expected to survive, only the field values) into the guarded client's
// Transport, and the base itself is left untouched (no shared mutable state
// between caller and guard).
func TestNewSSRFGuardedClient_TransportTierPreservesConfig(t *testing.T) {
	t.Parallel()
	const marker = "config-preserved-marker"
	base := &http.Client{
		Timeout: 7 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{ServerName: marker}, //nolint:gosec // test fixture, not a real TLS config
			MaxIdleConns:    42,
		},
	}
	guarded := NewSSRFGuardedClient(base)

	tr, ok := guarded.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", guarded.Transport)
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.ServerName != marker {
		t.Error("TLSClientConfig marker not preserved through Clone()")
	}
	if tr.MaxIdleConns != 42 {
		t.Errorf("MaxIdleConns = %d, want 42", tr.MaxIdleConns)
	}
	if tr.DialContext == nil {
		t.Error("DialContext not wired to the SSRF guard")
	}
	if guarded.Timeout != 7*time.Second {
		t.Errorf("client Timeout = %v, want 7s (base preserved)", guarded.Timeout)
	}
	baseTr, ok := base.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("base transport type changed unexpectedly: %T", base.Transport)
	}
	if baseTr.DialContext != nil {
		t.Error("NewSSRFGuardedClient mutated the caller's base Transport")
	}
}

// countingRoundTripper counts RoundTrip invocations and returns a stub
// error, so tests can assert delegation happened (or didn't) without
// needing a real network response.
type countingRoundTripper struct {
	calls int
}

func (c *countingRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	c.calls++
	return nil, errors.New("countingRoundTripper: stub, no real network")
}

// TestNewSSRFGuardedClient_OpaqueTierPreChecksBeforeDelegating proves the
// opaque-RoundTripper tier refuses a blocked target WITHOUT ever calling the
// delegate (call-count == 0 on block), and DOES call the delegate for an
// allowed target — the pre-check-before-delegating contract for a
// stealth/fingerprint-evasion client this package cannot dial-guard
// directly.
func TestNewSSRFGuardedClient_OpaqueTierPreChecksBeforeDelegating(t *testing.T) {
	t.Parallel()
	delegate := &countingRoundTripper{}
	guarded := NewSSRFGuardedClient(&http.Client{Transport: delegate})
	ctx := context.Background()

	blockedReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1/blocked", nil)
	if err != nil {
		t.Fatalf("build blocked request: %v", err)
	}
	blockedResp, doErr := guarded.Do(blockedReq)
	if blockedResp != nil {
		blockedResp.Body.Close() //nolint:errcheck
	}
	if doErr == nil {
		t.Fatal("expected the opaque tier to refuse a blocked target")
	}
	if !errors.Is(doErr, ErrSSRFBlocked) {
		t.Errorf("expected error to wrap ErrSSRFBlocked, got: %v", doErr)
	}
	if delegate.calls != 0 {
		t.Errorf("delegate RoundTrip called %d times on a blocked target, want 0 (pre-check must block before delegating)", delegate.calls)
	}

	allowedReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://8.8.8.8/allowed", nil)
	if err != nil {
		t.Fatalf("build allowed request: %v", err)
	}
	// The delegate's RoundTrip always returns a stub error (no real network
	// call) — only the call-count matters here, so the returned error is
	// intentionally discarded.
	allowedResp, _ := guarded.Do(allowedReq)
	if allowedResp != nil {
		allowedResp.Body.Close() //nolint:errcheck
	}
	if delegate.calls != 1 {
		t.Errorf("delegate RoundTrip called %d times after an allowed target, want 1", delegate.calls)
	}
}

// TestNewSSRFGuardedClient_NilBase proves the nil-base path doesn't panic
// and is itself guarded.
func TestNewSSRFGuardedClient_NilBase(t *testing.T) {
	t.Parallel()
	guarded := NewSSRFGuardedClient(nil)
	if guarded == nil {
		t.Fatal("NewSSRFGuardedClient(nil) returned nil")
	}
	tr, ok := guarded.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", guarded.Transport)
	}
	if tr.DialContext == nil {
		t.Error("nil-base client Transport not guarded")
	}
}
