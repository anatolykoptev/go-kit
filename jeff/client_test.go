package jeff

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
		// Literal wire JSON in the shape the jeff server emits — encoding a
		// Response here would round-trip any struct-tag typo unnoticed.
		_, _ = io.WriteString(w, `{"model":"m","answers":{`+
			`"pick":{"type":"choice","choice":"billing","confidence":0.9,"probabilities":{"billing":0.9,"other":0.1}},`+
			`"level":{"type":"score","score":1.6,"confidence":0.7,"legend":{"0":"low","1":"mid","2":"high"},`+
			`"probabilities":{"0":0.1,"1":0.2,"2":0.7}}},`+
			`"usage":{"input_tokens":64,"output_tokens":6}}`)
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
	if got := resp.Answers["level"].Score; got != 1.6 {
		t.Fatalf("score = %v, want 1.6", got)
	}
	if lvl, ok := resp.Answers["level"].Level(); !ok || lvl != 2 {
		t.Fatalf("Level() = %d, %v; want 2, true", lvl, ok)
	}
}

// TestScoreLevelIsArgmaxNotMean decodes score answers and checks Level
// picks the argmax of Probabilities rather than rounding or truncating the
// fractional Score. The first case is a live capture from
// gliformer-large-v1; the others use the server's formula (Score over the
// raw distribution, Probabilities = raw^(1/3.2) renormalized).
func TestScoreLevelIsArgmaxNotMean(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"live capture", `{"type":"score","score":0.0197,"confidence":0.5968,` +
			`"legend":{"0":"low","1":"medium","2":"high"},` +
			`"probabilities":{"0":0.7312,"1":0.1005,"2":0.1683}}`, 0},
		// raw {0.6, 0, 0.4}: Score 0.8 rounds to level 1, which has p=0.
		{"bimodal mean between levels", `{"type":"score","score":0.8,"confidence":0.3,` +
			`"probabilities":{"0":0.5316,"1":0.0,"2":0.4684}}`, 0},
		// raw {0.05, 0.15, 0.8}: Score 1.75 truncates to level 1.
		{"top level", `{"type":"score","score":1.75,"confidence":0.35,` +
			`"probabilities":{"0":0.2089,"1":0.2944,"2":0.4967}}`, 2},
		// all-zero raw scores: the server emits a uniform distribution.
		{"uniform tie resolves to lowest", `{"type":"score","score":1.0,"confidence":0,` +
			`"probabilities":{"0":0.3333,"1":0.3333,"2":0.3333}}`, 0},
		{"tie between upper levels", `{"type":"score","score":1.5,"confidence":0.25,` +
			`"probabilities":{"0":0.0,"1":0.5,"2":0.5}}`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a Answer
			if err := json.Unmarshal([]byte(tc.raw), &a); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			// Repeat: map iteration order varies, a missing tie-break flakes.
			for range 50 {
				got, ok := a.Level()
				if !ok || got != tc.want {
					t.Fatalf("Level() = %d, %v; want %d, true (Score=%v)", got, ok, tc.want, a.Score)
				}
			}
		})
	}
}

func TestLevelRejectsNonScoreAnswers(t *testing.T) {
	cases := []struct {
		name string
		a    Answer
	}{
		{"score without probabilities", Answer{Type: "score", Score: 1}},
		{"choice with numeric option names", Answer{Type: "choice", Choice: "5",
			Probabilities: map[string]float64{"1": 0.1, "5": 0.9}}},
		{"choice with mixed option names", Answer{Type: "choice", Choice: "other",
			Probabilities: map[string]float64{"0": 0.2, "other": 0.8}}},
		{"score with non-index keys", Answer{Type: "score",
			Probabilities: map[string]float64{"low": 1}}},
	}
	for _, tc := range cases {
		if lvl, ok := tc.a.Level(); ok {
			t.Errorf("%s: Level() = %d, true; want ok=false", tc.name, lvl)
		}
	}
}
