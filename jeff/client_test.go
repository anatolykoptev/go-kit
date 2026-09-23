package jeff

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(t *testing.T, prob float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer testkey" {
			t.Errorf("Authorization = %q, want Bearer testkey", r.Header.Get("Authorization"))
		}
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Response{
			Model:   "gliformer-large-v1",
			Answers: map[string]Answer{"q": {Type: "noul", Noul: prob}},
			Usage:   Usage{InputTokens: 64, OutputTokens: 6},
		})
	}))
}

func TestAskNoulParsesProbability(t *testing.T) {
	srv := newTestServer(t, 0.83)
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("testkey"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	prob, err := c.AskNoul(context.Background(), map[string]any{"url": "x"}, "ok?")
	if err != nil {
		t.Fatalf("AskNoul: %v", err)
	}
	if prob != 0.83 {
		t.Fatalf("prob = %v, want 0.83", prob)
	}
}

func TestAskNoulAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("bad"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	var statusErr *StatusError
	if _, err := c.AskNoul(context.Background(), nil, "ok?"); !errors.As(err, &statusErr) || statusErr.StatusCode != 401 {
		t.Fatalf("err = %v, want StatusError 401", err)
	}
}

func TestAskNoulMissingAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Response{Answers: map[string]Answer{}})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("testkey"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.AskNoul(context.Background(), nil, "ok?"); !errors.Is(err, ErrNoAnswer) {
		t.Fatalf("err = %v, want ErrNoAnswer", err)
	}
}

func TestRequireAuthFailsConstruction(t *testing.T) {
	t.Setenv("JEFF_TOKEN", "")
	if _, err := NewClient("http://127.0.0.1:1", WithRequireAuth()); !errors.Is(err, ErrNoToken) {
		t.Fatalf("err = %v, want ErrNoToken", err)
	}
}

func TestEnvTokenResolution(t *testing.T) {
	t.Setenv("JEFF_TOKEN", "envkey")
	c, err := NewClient("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.token != "envkey" {
		t.Fatalf("token = %q, want envkey", c.token)
	}
}

func TestAskDecodesChoiceAndScore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Response{
			Model: "m",
			Answers: map[string]Answer{
				"pick":  {Type: "choice", Choice: "billing", Confidence: 0.9, Probabilities: map[string]float64{"billing": 0.9, "other": 0.1}},
				"level": {Type: "score", Score: 2, Confidence: 0.7, Legend: map[string]any{"0": "low"}},
			},
		})
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, WithToken("testkey"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	resp, err := c.Ask(context.Background(), Request{
		State: map[string]any{"text": "refund me"},
		Questions: map[string]Question{
			"pick":  ChoiceQuestion("which team?", map[string]any{"billing": nil, "other": nil}),
			"level": ScoreQuestion("how urgent?", []any{"low", "mid", "high"}),
		},
	})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if resp.Answers["pick"].Choice != "billing" {
		t.Fatalf("choice = %q, want billing", resp.Answers["pick"].Choice)
	}
	if resp.Answers["level"].Score != 2 {
		t.Fatalf("score = %v, want 2", resp.Answers["level"].Score)
	}
}
