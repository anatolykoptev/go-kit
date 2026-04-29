package rerank

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// newFallbackTestClient builds a Client against the given httptest server URL
// with NoRetry so fallback tests are fast and deterministic.
func newFallbackTestClient(t *testing.T, url string) *Client {
	t.Helper()
	return NewClient(url,
		WithTimeout(500*time.Millisecond),
		WithRetry(NoRetry), // no retry in fallback tests — fallback path tested separately
	)
}

// TestFallback_PrimarySuccessNoSecondaryCall verifies that when primary
// succeeds, the secondary is never called.
func TestFallback_PrimarySuccessNoSecondaryCall(t *testing.T) {
	secondaryCalls := 0
	primarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		v1JSONResp(w, cohereResponse{
			Results: []cohereResult{{Index: 0, RelevanceScore: 0.9}},
		})
	})
	secondarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		secondaryCalls++
		v1JSONResp(w, cohereResponse{
			Results: []cohereResult{{Index: 0, RelevanceScore: 0.5}},
		})
	})

	primary := newFallbackTestClient(t, primarySrv.URL)
	secondary := newFallbackTestClient(t, secondarySrv.URL)

	docs := []Doc{{ID: "a", Text: "x"}}
	res := rerankWithFallback(context.Background(), primary, secondary, "q", docs)

	if res.Status != StatusOk {
		t.Errorf("Status: got %v want StatusOk", res.Status)
	}
	if secondaryCalls != 0 {
		t.Errorf("secondary calls: got %d want 0", secondaryCalls)
	}
}

// TestFallback_PrimaryDegradedSecondaryCalled verifies that on primary 503,
// secondary is called and StatusFallback is returned.
func TestFallback_PrimaryDegradedSecondaryCalled(t *testing.T) {
	secondaryCalls := 0
	primarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	secondarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		secondaryCalls++
		v1JSONResp(w, cohereResponse{
			Results: []cohereResult{{Index: 0, RelevanceScore: 0.7}},
		})
	})

	primary := newFallbackTestClient(t, primarySrv.URL)
	secondary := newFallbackTestClient(t, secondarySrv.URL)

	docs := []Doc{{ID: "a", Text: "x"}}
	res := rerankWithFallback(context.Background(), primary, secondary, "q", docs)

	if res.Status != StatusFallback {
		t.Errorf("Status: got %v want StatusFallback", res.Status)
	}
	if secondaryCalls != 1 {
		t.Errorf("secondary calls: got %d want 1", secondaryCalls)
	}
	if len(res.Scored) == 0 {
		t.Error("expected non-empty Scored from secondary")
	}
}

// TestFallback_PrimaryClientErrorNoSecondary verifies that on primary 400,
// the secondary is NOT called (4xx = caller error).
func TestFallback_PrimaryClientErrorNoSecondary(t *testing.T) {
	secondaryCalls := 0
	primarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	secondarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		secondaryCalls++
		v1JSONResp(w, cohereResponse{})
	})

	primary := newFallbackTestClient(t, primarySrv.URL)
	secondary := newFallbackTestClient(t, secondarySrv.URL)

	docs := []Doc{{ID: "a"}, {ID: "b"}}
	res := rerankWithFallback(context.Background(), primary, secondary, "q", docs)

	if res.Status != StatusDegraded {
		t.Errorf("Status: got %v want StatusDegraded", res.Status)
	}
	if secondaryCalls != 0 {
		t.Errorf("secondary calls: got %d want 0 (4xx must not trigger fallback)", secondaryCalls)
	}
}

// TestFallback_BothFail verifies that when both primary and secondary fail,
// the primary's Degraded result is returned.
func TestFallback_BothFail(t *testing.T) {
	primarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	secondarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	primary := newFallbackTestClient(t, primarySrv.URL)
	secondary := newFallbackTestClient(t, secondarySrv.URL)

	docs := []Doc{{ID: "a"}, {ID: "b"}}
	res := rerankWithFallback(context.Background(), primary, secondary, "q", docs)

	if res.Status != StatusDegraded {
		t.Errorf("Status: got %v want StatusDegraded", res.Status)
	}
	if res.Err == nil {
		t.Error("Err: expected non-nil on double failure")
	}
	// Passthrough preserves original order.
	if len(res.Scored) != 2 || res.Scored[0].ID != "a" {
		t.Errorf("passthrough order broken: %+v", res.Scored)
	}
}

// TestFallback_NilSecondaryDegraded verifies that a nil secondary returns
// the primary's Degraded result unchanged.
func TestFallback_NilSecondaryDegraded(t *testing.T) {
	primarySrv := newTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	primary := newFallbackTestClient(t, primarySrv.URL)

	docs := []Doc{{ID: "a"}}
	res := rerankWithFallback(context.Background(), primary, nil, "q", docs)

	if res.Status != StatusDegraded {
		t.Errorf("Status: got %v want StatusDegraded (nil secondary)", res.Status)
	}
}

// TestIsClientError verifies the 4xx detection helper.
func TestIsClientError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{errHTTPStatus{Code: 400}, true},
		{errHTTPStatus{Code: 404}, true},
		{errHTTPStatus{Code: 429}, true},
		{errHTTPStatus{Code: 499}, true},
		{errHTTPStatus{Code: 500}, false},
		{errHTTPStatus{Code: 503}, false},
		{errHTTPStatus{Code: 200}, false},
		{nil, false},
	}
	for _, tc := range cases {
		got := isClientError(tc.err)
		if got != tc.want {
			t.Errorf("isClientError(%v): got %v want %v", tc.err, got, tc.want)
		}
	}
}
