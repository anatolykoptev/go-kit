package httpmw

import (
	"net/http"
	"testing"
)

// stdlibFormatter must produce "<method> <path>" once, regardless of whether
// the Go 1.22 ServeMux pattern was registered with or without a method
// qualifier.
func TestStdlibFormatter_NoMethodDuplication(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		pattern string
		want    string
	}{
		{
			name:    "method-qualified pattern (POST /webhook)",
			method:  "POST",
			pattern: "POST /webhook",
			want:    "POST /webhook",
		},
		{
			name:    "method-qualified pattern with trailing slash",
			method:  "POST",
			pattern: "POST /webhook/",
			want:    "POST /webhook/",
		},
		{
			name:    "bare pattern",
			method:  "GET",
			pattern: "/health",
			want:    "GET /health",
		},
		{
			name:    "bare pattern with path variable",
			method:  "GET",
			pattern: "/users/{id}",
			want:    "GET /users/{id}",
		},
		{
			name:    "method-qualified with path variable",
			method:  "DELETE",
			pattern: "DELETE /users/{id}",
			want:    "DELETE /users/{id}",
		},
		{
			name:    "empty pattern (no route matched)",
			method:  "GET",
			pattern: "",
			want:    "GET unmatched",
		},
		{
			name:    "different method in pattern (should not be treated as duplicate)",
			method:  "GET",
			pattern: "POST /webhook",
			want:    "GET POST /webhook",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &http.Request{Method: tc.method, Pattern: tc.pattern}
			got := stdlibFormatter("svc", r)
			if got != tc.want {
				t.Errorf("stdlibFormatter(%q, %q) = %q; want %q",
					tc.method, tc.pattern, got, tc.want)
			}
		})
	}
}
