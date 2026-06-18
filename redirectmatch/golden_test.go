// Package redirectmatch_test runs cross-language contract tests from testdata/golden.json.
// The golden file is the shared contract mirrored by the piter-now TypeScript resolver.
package redirectmatch_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/anatolykoptev/go-kit/redirectmatch"
)

// goldenFile mirrors the JSON schema defined in testdata/golden.json.
type goldenFile struct {
	Policy struct {
		StripTrailingSlash bool `json:"strip_trailing_slash"`
		LowerCase          bool `json:"lower_case"`
		DecodeOnce         bool `json:"decode_once"`
	} `json:"policy"`
	Rules []goldenRule `json:"rules"`
	Cases []goldenCase `json:"cases"`
}

type goldenRule struct {
	ID            int64  `json:"id"`
	SourcePath    string `json:"source_path"`
	MatchType     string `json:"match_type"`
	Target        string `json:"target"`
	StatusCode    int    `json:"status_code"`
	QueryHandling string `json:"query_handling"`
	Priority      int    `json:"priority"`
}

type goldenCase struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Query    string `json:"query"`
	Matched  bool   `json:"matched"`
	Status   int    `json:"status"`
	Location string `json:"location"`
}

func TestGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/golden.json")
	if err != nil {
		t.Fatalf("failed to read testdata/golden.json: %v", err)
	}

	var gf goldenFile
	if err := json.Unmarshal(data, &gf); err != nil {
		t.Fatalf("failed to parse golden.json: %v", err)
	}

	// Build policy
	p := redirectmatch.Policy{
		StripTrailingSlash: gf.Policy.StripTrailingSlash,
		LowerCase:          gf.Policy.LowerCase,
		DecodeOnce:         gf.Policy.DecodeOnce,
	}

	// Build specs from golden rules
	specs := make([]redirectmatch.RuleSpec, len(gf.Rules))
	for i, gr := range gf.Rules {
		specs[i] = redirectmatch.RuleSpec{
			ID:            gr.ID,
			SourcePath:    gr.SourcePath,
			MatchType:     redirectmatch.MatchType(gr.MatchType),
			Target:        gr.Target,
			StatusCode:    gr.StatusCode,
			QueryHandling: redirectmatch.QueryMode(gr.QueryHandling),
			Priority:      gr.Priority,
		}
	}

	set, compileErrs := redirectmatch.BuildRuleSet(specs, p)
	if len(compileErrs) > 0 {
		t.Fatalf("BuildRuleSet: compile errors on golden rules: %v", compileErrs)
	}

	for _, tc := range gf.Cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			dec := redirectmatch.Resolve(set, tc.Path, tc.Query)
			if dec.Matched != tc.Matched {
				t.Errorf("Matched = %v, want %v", dec.Matched, tc.Matched)
			}
			if tc.Matched {
				if dec.StatusCode != tc.Status {
					t.Errorf("StatusCode = %d, want %d", dec.StatusCode, tc.Status)
				}
				if dec.Location != tc.Location {
					t.Errorf("Location = %q, want %q", dec.Location, tc.Location)
				}
			}
		})
	}
}
