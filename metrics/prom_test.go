package metrics

import "testing"

func TestParseLabeled(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantKeys []string
		wantVals []string
	}{
		{"wp_rest_calls", "wp_rest_calls", nil, nil},
		{"rpc{method=login}", "rpc", []string{"method"}, []string{"login"}},
		{"rpc{service=auth,method=login}", "rpc", []string{"service", "method"}, []string{"auth", "login"}},
		{"malformed{", "malformed{", nil, nil}, // invalid → treat as plain
	}
	for _, c := range cases {
		gotName, gotKeys, gotVals := parseLabeled(c.in)
		if gotName != c.wantName {
			t.Errorf("%q: name = %q, want %q", c.in, gotName, c.wantName)
		}
		if len(gotKeys) != len(c.wantKeys) {
			t.Errorf("%q: keys = %v, want %v", c.in, gotKeys, c.wantKeys)
			continue
		}
		for i := range gotKeys {
			if gotKeys[i] != c.wantKeys[i] || gotVals[i] != c.wantVals[i] {
				t.Errorf("%q: [%d] = (%q,%q), want (%q,%q)", c.in, i, gotKeys[i], gotVals[i], c.wantKeys[i], c.wantVals[i])
			}
		}
	}
}
