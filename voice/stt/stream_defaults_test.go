package stt

import (
	"net/url"
	"strings"
	"testing"
)

// TestStreamParamsDefaultsApplied verifies that VAD and Punctuate default to
// true when not explicitly set (nil *bool), matching the doc comments. Before
// the fix, buildStreamURL sent the zero value (false) for both fields.
func TestStreamParamsDefaultsApplied(t *testing.T) {
	got, err := buildStreamURL("http://host:8092", StreamParams{})
	if err != nil {
		t.Fatalf("buildStreamURL: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse result URL: %v", err)
	}
	q := u.Query()
	if q.Get("vad") != "true" {
		t.Errorf("vad: want %q, got %q", "true", q.Get("vad"))
	}
	if q.Get("punctuate") != "true" {
		t.Errorf("punctuate: want %q, got %q", "true", q.Get("punctuate"))
	}
}

// TestStreamParamsExplicitFalseRespected verifies that explicitly setting
// VAD=false and Punctuate=false is honored (not overridden by the default).
func TestStreamParamsExplicitFalseRespected(t *testing.T) {
	got, err := buildStreamURL("http://host:8092", StreamParams{
		VAD:       Bool(false),
		Punctuate: Bool(false),
	})
	if err != nil {
		t.Fatalf("buildStreamURL: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse result URL: %v", err)
	}
	q := u.Query()
	if q.Get("vad") != "false" {
		t.Errorf("vad: want %q, got %q", "false", q.Get("vad"))
	}
	if q.Get("punctuate") != "false" {
		t.Errorf("punctuate: want %q, got %q", "false", q.Get("punctuate"))
	}
}

// TestStreamParamsExplicitTrueRespected verifies that explicitly setting
// VAD=true and Punctuate=true is honored.
func TestStreamParamsExplicitTrueRespected(t *testing.T) {
	got, err := buildStreamURL("http://host:8092", StreamParams{
		VAD:       Bool(true),
		Punctuate: Bool(true),
	})
	if err != nil {
		t.Fatalf("buildStreamURL: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse result URL: %v", err)
	}
	q := u.Query()
	if q.Get("vad") != "true" {
		t.Errorf("vad: want %q, got %q", "true", q.Get("vad"))
	}
	if !strings.Contains(got, "vad=true") {
		t.Errorf("URL should contain vad=true, got: %s", got)
	}
}
