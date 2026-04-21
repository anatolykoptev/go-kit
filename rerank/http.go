package rerank

// cohereRequest mirrors the Cohere /v1/rerank request body (also accepted by
// embed-server, TEI, Jina, Voyage, Mixedbread).
type cohereRequest struct {
	Model     string   `json:"model,omitempty"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      *int     `json:"top_n,omitempty"`
}

// cohereResult is a single scored doc in the rerank response.
type cohereResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// cohereResponse is the full rerank response body.
type cohereResponse struct {
	Model   string         `json:"model"`
	Results []cohereResult `json:"results"`
}
