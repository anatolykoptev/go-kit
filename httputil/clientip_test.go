package httputil_test

import (
	"net/http"
	"testing"

	"github.com/anatolykoptev/go-kit/httputil"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xRealIP    string
		xff        string
		want       string
	}{
		{
			name:       "X-Real-IP valid returns normalized IP",
			remoteAddr: "127.0.0.1:9999",
			xRealIP:    "203.0.113.5",
			want:       "203.0.113.5",
		},
		{
			name:       "X-Real-IP invalid falls through to XFF",
			remoteAddr: "127.0.0.1:9999",
			xRealIP:    "not-an-ip",
			xff:        "198.51.100.7",
			want:       "198.51.100.7",
		},
		{
			name:       "X-Real-IP invalid and XFF invalid falls through to RemoteAddr",
			remoteAddr: "127.0.0.1:9999",
			xRealIP:    "bad-ip",
			xff:        "also-bad",
			want:       "127.0.0.1",
		},
		{
			name:       "XFF first hop valid returned when no X-Real-IP",
			remoteAddr: "127.0.0.1:9999",
			xff:        "203.0.113.9, 10.0.0.1, 10.0.0.2",
			want:       "203.0.113.9",
		},
		{
			name:       "XFF invalid falls through to RemoteAddr",
			remoteAddr: "127.0.0.1:9999",
			xff:        "spoofed-value",
			want:       "127.0.0.1",
		},
		{
			name:       "No headers returns RemoteAddr host",
			remoteAddr: "203.0.113.1:55234",
			want:       "203.0.113.1",
		},
		{
			name:       "IPv6 RemoteAddr brackets stripped",
			remoteAddr: "[2001:db8::1]:1234",
			want:       "2001:db8::1",
		},
		{
			name:       "X-Real-IP with whitespace trimmed and validated",
			remoteAddr: "127.0.0.1:9999",
			xRealIP:    "  203.0.113.42  ",
			want:       "203.0.113.42",
		},
		{
			// net.ParseIP("::ffff:203.0.113.5").String() returns "203.0.113.5" in Go —
			// the IPv4-mapped form is normalized to plain IPv4.
			name:       "ip.String() normalizes IPv4-mapped IPv6 to plain IPv4",
			remoteAddr: "127.0.0.1:9999",
			xRealIP:    "::ffff:203.0.113.5",
			want:       "203.0.113.5",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.RemoteAddr = tc.remoteAddr
			if tc.xRealIP != "" {
				req.Header.Set("X-Real-IP", tc.xRealIP)
			}
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}

			got := httputil.ClientIP(req)
			if got != tc.want {
				t.Errorf("ClientIP() = %q, want %q", got, tc.want)
			}
		})
	}
}
