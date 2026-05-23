package llm_test

import (
	"reflect"
	"testing"

	"github.com/anatolykoptev/go-kit/llm"
)

func TestParseModelFallbackChain(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "gemini-3.1-flash-lite-preview", []string{"gemini-3.1-flash-lite-preview"}},
		{"csv with spaces", "a, b ,c", []string{"a", "b", "c"}},
		{"drop empty tokens", "a,,b,,,c", []string{"a", "b", "c"}},
		{"dedup", "a,b,a,c,b", []string{"a", "b", "c"}},
		{"sanitize drops unsafe", "good-model,bad model,with@at,clean_one,with/slash", []string{"good-model", "clean_one", "with/slash"}},
		{
			"real llm.env chain",
			"gemini-3-flash-preview,cerebras-qwen-3-235b,groq-llama-70b,cerebras-glm-4.7,or-deepseek-v4-flash-free,or-gpt-oss-120b-free,or-minimax-m2.5-free",
			[]string{
				"gemini-3-flash-preview", "cerebras-qwen-3-235b", "groq-llama-70b",
				"cerebras-glm-4.7", "or-deepseek-v4-flash-free", "or-gpt-oss-120b-free",
				"or-minimax-m2.5-free",
			},
		},
		{"all whitespace", "  ,  ,  ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llm.ParseModelFallbackChain(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseModelFallbackChain(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildModelChainEndpoints(t *testing.T) {
	const (
		url = "http://cliproxyapi:8317/v1"
		key = "sk-test"
	)
	tests := []struct {
		name    string
		primary string
		chain   []string
		want    []llm.Endpoint
	}{
		{
			name:    "empty chain only primary",
			primary: "gemini-3.1-flash-lite-preview",
			chain:   nil,
			want: []llm.Endpoint{
				{URL: url, Key: key, Model: "gemini-3.1-flash-lite-preview"},
			},
		},
		{
			name:    "primary + chain",
			primary: "gemini-3.1-flash-lite-preview",
			chain:   []string{"cerebras-qwen-3-235b", "groq-llama-70b"},
			want: []llm.Endpoint{
				{URL: url, Key: key, Model: "gemini-3.1-flash-lite-preview"},
				{URL: url, Key: key, Model: "cerebras-qwen-3-235b"},
				{URL: url, Key: key, Model: "groq-llama-70b"},
			},
		},
		{
			name:    "chain contains primary — drop dup",
			primary: "a",
			chain:   []string{"b", "a", "c"},
			want: []llm.Endpoint{
				{URL: url, Key: key, Model: "a"},
				{URL: url, Key: key, Model: "b"},
				{URL: url, Key: key, Model: "c"},
			},
		},
		{
			name:    "chain duplicates internal",
			primary: "a",
			chain:   []string{"b", "b", "c", "c"},
			want: []llm.Endpoint{
				{URL: url, Key: key, Model: "a"},
				{URL: url, Key: key, Model: "b"},
				{URL: url, Key: key, Model: "c"},
			},
		},
		{
			name:    "empty primary — chain only",
			primary: "",
			chain:   []string{"a", "b"},
			want: []llm.Endpoint{
				{URL: url, Key: key, Model: "a"},
				{URL: url, Key: key, Model: "b"},
			},
		},
		{
			name:    "empty tokens in chain skipped",
			primary: "a",
			chain:   []string{"", "b", "", "c"},
			want: []llm.Endpoint{
				{URL: url, Key: key, Model: "a"},
				{URL: url, Key: key, Model: "b"},
				{URL: url, Key: key, Model: "c"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llm.BuildModelChainEndpoints(url, key, tt.primary, tt.chain)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildModelChainEndpoints_IntegratesWithWithEndpoints(t *testing.T) {
	// Регрессия: убедиться что результат Build* реально используется
	// WithEndpoints (тип одинаковый, поля совпадают).
	eps := llm.BuildModelChainEndpoints("http://x/v1", "k", "a", []string{"b"})
	c := llm.NewClient("ignored", "ignored", "ignored-model", llm.WithEndpoints(eps))
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	// Конструкция не паникует — функциональный smoke-тест полной интеграции
	// в TestEndpoints_* в client_test.go.
}
