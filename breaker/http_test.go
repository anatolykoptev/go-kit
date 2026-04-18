// breaker/http_test.go
package breaker

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPDoer_TripsOn5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := New(Options{FailThreshold: 2, OpenDuration: 100 * time.Millisecond})
	d := NewHTTPDoer(http.DefaultClient, b)
	for range 2 {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		_, _ = d.Do(req)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	_, err := d.Do(req)
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("err = %v, want ErrOpen", err)
	}
}

func TestHTTPDoer_DoesNotTripOn2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := New(Options{FailThreshold: 2, OpenDuration: time.Second})
	d := NewHTTPDoer(http.DefaultClient, b)
	for range 5 {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		resp, err := d.Do(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp.Body.Close()
	}
	if b.State() != StateClosed {
		t.Fatalf("state = %s, want closed", b.State())
	}
}
