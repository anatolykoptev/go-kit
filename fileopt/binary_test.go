package fileopt

import "testing"

func TestResolveBinary_EnvOverrideWins(t *testing.T) {
	t.Setenv("TEST_FAKE_BIN", "/custom/path/fakebin")
	got := resolveBinary("TEST_FAKE_BIN", "nonexistent-bin-xyz")
	if got != "/custom/path/fakebin" {
		t.Fatalf("env override: got %q, want /custom/path/fakebin", got)
	}
}

func TestResolveBinary_PATHLookup(t *testing.T) {
	t.Setenv("TEST_FAKE_BIN", "")
	got := resolveBinary("TEST_FAKE_BIN", "sh") // /bin/sh is guaranteed on Linux
	if got == "" {
		t.Fatalf("expected PATH lookup for sh to succeed, got empty")
	}
}

func TestResolveBinary_MissingReturnsEmpty(t *testing.T) {
	t.Setenv("TEST_FAKE_BIN", "")
	got := resolveBinary("TEST_FAKE_BIN", "definitely-not-installed-xyz-123")
	if got != "" {
		t.Fatalf("expected empty string for missing binary, got %q", got)
	}
}
