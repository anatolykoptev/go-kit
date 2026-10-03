package stt

import (
	"strings"
	"testing"
)

// TestConvertWebSocketScheme verifies the net/url-based scheme conversion that
// replaces the naive strings.NewReplacer approach. The naive replacer failed
// for schemeless loopback addresses, double-schemes, and mangled ws://
// passthrough. These rows go RED against the old string-replace guard.
func TestConvertWebSocketScheme(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		base    string
		want    string
		wantErr bool
	}{
		{name: "no scheme loopback", base: "127.0.0.1:8092", wantErr: true},
		{name: "http to ws", base: "http://host", want: "ws://host"},
		{name: "https to wss", base: "https://host", want: "wss://host"},
		{name: "ws passthrough", base: "ws://host", want: "ws://host"},
		{name: "wss passthrough", base: "wss://host", want: "wss://host"},
		{name: "trailing slash stripped no double slash", base: "http://host:8092/", want: "ws://host:8092"},
		{name: "query preserved", base: "https://host/path?q=1", want: "wss://host/path?q=1"},
		{name: "empty", base: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := convertWebSocketScheme(tc.base)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("convertWebSocketScheme(%q) = %q, want error", tc.base, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("convertWebSocketScheme(%q) err = %v, want nil", tc.base, err)
			}
			if got != tc.want {
				t.Errorf("convertWebSocketScheme(%q) = %q, want %q", tc.base, got, tc.want)
			}
		})
	}
}

// TestBuildStreamURLRejectsBadScheme confirms the shipped buildStreamURL path
// surfaces the scheme-conversion error rather than silently misrouting.
func TestBuildStreamURLRejectsBadScheme(t *testing.T) {
	t.Parallel()
	_, err := buildStreamURL("127.0.0.1:8092", StreamParams{})
	if err == nil {
		t.Fatal("expected error for schemeless URL")
	}
	if !strings.Contains(err.Error(), "scheme") && !strings.Contains(err.Error(), "invalid stream URL") {
		t.Errorf("err = %q, want to mention scheme/invalid stream URL", err.Error())
	}
}

// TestBuildStreamURLAppendsListenPath verifies the full URL keeps the
// /v1/listen endpoint and stream query params after scheme conversion.
func TestBuildStreamURLAppendsListenPath(t *testing.T) {
	t.Parallel()
	got, err := buildStreamURL("http://host:8092/", StreamParams{Language: "en"})
	if err != nil {
		t.Fatalf("buildStreamURL err = %v", err)
	}
	if !strings.HasPrefix(got, "ws://host:8092/v1/listen?") {
		t.Errorf("buildStreamURL = %q, want prefix ws://host:8092/v1/listen?", got)
	}
	// No double slash from the stripped trailing slash.
	if strings.Contains(got, "8092//v1/listen") {
		t.Errorf("buildStreamURL = %q, contains double slash", got)
	}
}
