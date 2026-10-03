package stt_test

import (
	"errors"
	"net/http"
	"testing"

	stt "github.com/anatolykoptev/go-kit/voice/stt"
)

func TestErrorIs(t *testing.T) {
	orig := &stt.Error{StatusCode: http.StatusInternalServerError, Message: "internal error"}
	var target *stt.Error
	if !errors.As(orig, &target) {
		t.Fatal("errors.As failed for *stt.Error")
	}
	if target.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want 500", target.StatusCode)
	}
	if target.Message != "internal error" {
		t.Errorf("Message = %q, want %q", target.Message, "internal error")
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		code      int
		transient bool
	}{
		{429, true},
		{502, true},
		{503, true},
		{504, true},
		{400, false},
		{500, false},
		{200, false},
		{401, false},
	}
	for _, tt := range tests {
		e := &stt.Error{StatusCode: tt.code}
		if got := e.IsTransient(); got != tt.transient {
			t.Errorf("IsTransient(%d) = %v, want %v", tt.code, got, tt.transient)
		}
	}
}

func TestErrorString(t *testing.T) {
	e := &stt.Error{StatusCode: 503, Message: "service unavailable"}
	want := "stt: HTTP 503: service unavailable"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
