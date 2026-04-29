package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestApplyInstructions_EmptyDefaults_NoOp(t *testing.T) {
	q, docs := applyInstructions("query", []string{"doc a", "doc b"}, "", "")
	if q != "query" {
		t.Errorf("query changed: got %q", q)
	}
	if docs[0] != "doc a" || docs[1] != "doc b" {
		t.Errorf("docs changed: got %v", docs)
	}
}

func TestApplyInstructions_QueryOnly(t *testing.T) {
	q, docs := applyInstructions("question", []string{"passage"}, "Represent:", "")
	if q != "Represent: question" {
		t.Errorf("query: got %q", q)
	}
	if docs[0] != "passage" {
		t.Errorf("doc changed unexpectedly: got %q", docs[0])
	}
}

func TestApplyInstructions_BothSet(t *testing.T) {
	q, docs := applyInstructions("q", []string{"d1", "d2"}, "QPrefix:", "DPrefix:")
	if q != "QPrefix: q" {
		t.Errorf("query: got %q", q)
	}
	for i, d := range docs {
		expected := "DPrefix: " + []string{"d1", "d2"}[i]
		if d != expected {
			t.Errorf("doc %d: got %q want %q", i, d, expected)
		}
	}
}

func TestWithInstruction_HTTPRequestBodyHasPrefix(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req cohereRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotQuery = req.Query
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cohereResponse{Results: []cohereResult{{Index: 0, RelevanceScore: 0.9}}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL,
		WithModel("test"),
		WithTimeout(time.Second),
		WithInstruction("SearchQuery:", ""),
	)
	docs := []Doc{{ID: "a", Text: "some passage"}}
	_, _ = c.RerankWithResult(context.Background(), "my question", docs)

	if gotQuery != "SearchQuery: my question" {
		t.Errorf("query in HTTP body: got %q, want %q", gotQuery, "SearchQuery: my question")
	}
}
