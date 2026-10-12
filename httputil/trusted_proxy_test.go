package httputil_test

import (
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/anatolykoptev/go-kit/httputil"
)

func mustProxies(t *testing.T, csv string) []netip.Prefix {
	t.Helper()
	p, err := httputil.ParseTrustedProxies(csv)
	if err != nil {
		t.Fatalf("ParseTrustedProxies(%q): %v", csv, err)
	}
	return p
}

func TestTrustedProxyClientIP(t *testing.T) {
	trusted := "10.0.0.0/8, 172.16.0.0/12, fd00::/8, 192.168.1.1"
	tests := []struct {
		name       string
		remoteAddr string
		xRealIP    string
		xff        []string
		want       string
	}{
		{"spoofed X-Real-IP ignored from untrusted peer", "203.0.113.9:4000", "6.6.6.6", nil, "203.0.113.9"},
		{"spoofed X-Real-IP ignored from trusted peer", "10.0.0.2:4000", "6.6.6.6", nil, "10.0.0.2"},
		{"XFF from untrusted peer ignored", "203.0.113.9:4000", "", []string{"6.6.6.6, 1.2.3.4"}, "203.0.113.9"},
		{"trusted peer, spoofed left hop skipped", "10.0.0.2:4000", "", []string{"6.6.6.6, 1.2.3.4"}, "1.2.3.4"},
		{"trusted peer, chain ending in trusted proxies", "10.0.0.2:4000", "", []string{"6.6.6.6, 1.2.3.4, 10.1.1.1, 172.16.0.5"}, "1.2.3.4"},
		{"all-trusted chain returns peer", "10.0.0.2:4000", "", []string{"10.1.1.1, 172.16.0.5"}, "10.0.0.2"},
		{"no XFF returns peer", "10.0.0.2:4000", "", nil, "10.0.0.2"},
		{"malformed hop before untrusted returns peer", "10.0.0.2:4000", "", []string{"1.2.3.4, garbage, 10.1.1.1"}, "10.0.0.2"},
		{"malformed rightmost hop returns peer", "10.0.0.2:4000", "", []string{"1.2.3.4, garbage"}, "10.0.0.2"},
		{"empty hop returns peer", "10.0.0.2:4000", "", []string{"1.2.3.4,, 10.1.1.1"}, "10.0.0.2"},
		{"multiple XFF lines walked right to left", "10.0.0.2:4000", "", []string{"6.6.6.6", "1.2.3.4, 10.1.1.1", "172.16.0.1"}, "1.2.3.4"},
		{"multiple XFF lines, later line untrusted wins", "10.0.0.2:4000", "", []string{"1.2.3.4", "5.6.7.8"}, "5.6.7.8"},
		{"IPv6 peer untrusted", "[2001:db8::1]:4000", "", []string{"1.2.3.4"}, "2001:db8::1"},
		{"IPv6 trusted peer and IPv6 hop", "[fd00::2]:4000", "", []string{"6.6.6.6, 2001:db8::7, fd00::9"}, "2001:db8::7"},
		{"IPv4-mapped trusted peer", "[::ffff:10.0.0.2]:4000", "", []string{"1.2.3.4"}, "1.2.3.4"},
		{"IPv4-mapped hop treated as trusted", "10.0.0.2:4000", "", []string{"1.2.3.4, ::ffff:10.1.1.1"}, "1.2.3.4"},
		{"IPv4-mapped untrusted hop returned unmapped", "10.0.0.2:4000", "", []string{"::ffff:1.2.3.4"}, "1.2.3.4"},
		{"bare-IP trusted entry", "192.168.1.1:80", "", []string{"1.2.3.4"}, "1.2.3.4"},
		{"zoned IPv6 hop zone stripped", "[fd00::2]:4000", "", []string{"fe80::1%x"}, "fe80::1"},
		{"untrusted IPv4-mapped peer unmapped", "[::ffff:203.0.113.9]:1", "", nil, "203.0.113.9"},
		{"untrusted zoned peer zone stripped", "[fe80::1%eth0]:1", "", nil, "fe80::1"},
		{"RemoteAddr without port", "10.0.0.2", "", []string{"1.2.3.4"}, "1.2.3.4"},
	}
	fn := httputil.TrustedProxyClientIP(mustProxies(t, trusted))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tc.remoteAddr, Header: http.Header{}}
			if tc.xRealIP != "" {
				r.Header.Set("X-Real-IP", tc.xRealIP)
			}
			for _, l := range tc.xff {
				r.Header.Add("X-Forwarded-For", l)
			}
			if got := fn(r); got != tc.want {
				t.Errorf("TrustedProxyClientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTrustedProxyClientIP_UnparseablePeerVerbatim(t *testing.T) {
	fn := httputil.TrustedProxyClientIP(mustProxies(t, "10.0.0.0/8"))
	r := &http.Request{RemoteAddr: "@", Header: http.Header{}}
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := fn(r); got != "@" {
		t.Errorf("got %q, want verbatim \"@\"", got)
	}
}

func TestTrustedProxyClientIP_EmptyTrustedIgnoresHeaders(t *testing.T) {
	fn := httputil.TrustedProxyClientIP(nil)
	r := &http.Request{RemoteAddr: "10.0.0.2:1", Header: http.Header{}}
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Real-IP", "5.6.7.8")
	if got := fn(r); got != "10.0.0.2" {
		t.Errorf("got %q, want peer 10.0.0.2", got)
	}
}

func TestParseTrustedProxies(t *testing.T) {
	ok := []struct {
		name, csv string
		want      []string
	}{
		{"cidrs", "10.0.0.0/8,fd00::/8", []string{"10.0.0.0/8", "fd00::/8"}},
		{"bare v4 becomes /32", "192.168.1.1", []string{"192.168.1.1/32"}},
		{"bare v6 becomes /128", "2001:db8::1", []string{"2001:db8::1/128"}},
		{"spaces and empty entries skipped", " 10.0.0.0/8 , ,, 1.2.3.4 ,", []string{"10.0.0.0/8", "1.2.3.4/32"}},
		{"mapped bare IP unmapped", "::ffff:10.0.0.1", []string{"10.0.0.1/32"}},
		{"mapped CIDR unmapped", "::ffff:10.0.0.0/104", []string{"10.0.0.0/8"}},
	}
	for _, tc := range ok {
		t.Run(tc.name, func(t *testing.T) {
			got, err := httputil.ParseTrustedProxies(tc.csv)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i, p := range got {
				if p.String() != tc.want[i] {
					t.Errorf("entry %d = %s, want %s", i, p, tc.want[i])
				}
			}
		})
	}
	bad := []struct{ name, csv, mention string }{
		{"garbage", "10.0.0.0/8,nope", "nope"},
		{"bad mask", "10.0.0.0/99", "10.0.0.0/99"},
		{"hostname", "proxy.local", "proxy.local"},
		{"empty string", "", "no trusted proxies"},
		{"only separators", " , ", "no trusted proxies"},
	}
	for _, tc := range bad {
		t.Run("invalid "+tc.name, func(t *testing.T) {
			got, err := httputil.ParseTrustedProxies(tc.csv)
			if err == nil {
				t.Fatalf("expected error, got %v", got)
			}
			if !strings.Contains(err.Error(), tc.mention) {
				t.Errorf("error %q does not name %q", err, tc.mention)
			}
		})
	}
}
