package rerank

import (
	"net/http"
	"testing"
	"time"
)

func TestDefaultCfg_Defaults(t *testing.T) {
	cfg := defaultCfg()
	if cfg.maxDocs != defaultMaxDocs {
		t.Errorf("maxDocs: got %d want %d", cfg.maxDocs, defaultMaxDocs)
	}
	if cfg.timeout != 0 {
		t.Errorf("timeout: got %v want 0 (no timeout)", cfg.timeout)
	}
	if cfg.observer == nil {
		t.Error("observer must not be nil (noopObserver expected)")
	}
	if cfg.hc == nil {
		t.Error("hc must not be nil (default http.Client expected)")
	}
	if cfg.url != "" {
		t.Errorf("url: got %q want empty", cfg.url)
	}
	if cfg.model != "" {
		t.Errorf("model: got %q want empty", cfg.model)
	}
	if cfg.apiKey != "" {
		t.Errorf("apiKey: got %q want empty", cfg.apiKey)
	}
	if cfg.maxCharsPerDoc != 0 {
		t.Errorf("maxCharsPerDoc: got %d want 0 (disabled)", cfg.maxCharsPerDoc)
	}
}

func TestWithModel_Applies(t *testing.T) {
	cfg := defaultCfg()
	WithModel("bge-reranker-v2-m3")(cfg)
	if cfg.model != "bge-reranker-v2-m3" {
		t.Errorf("model: got %q want %q", cfg.model, "bge-reranker-v2-m3")
	}
}

func TestWithAPIKey_Applies(t *testing.T) {
	cfg := defaultCfg()
	WithAPIKey("tok-abc")(cfg)
	if cfg.apiKey != "tok-abc" {
		t.Errorf("apiKey: got %q want %q", cfg.apiKey, "tok-abc")
	}
}

func TestWithTimeout_Applies(t *testing.T) {
	cfg := defaultCfg()
	WithTimeout(3 * time.Second)(cfg)
	if cfg.timeout != 3*time.Second {
		t.Errorf("timeout: got %v want 3s", cfg.timeout)
	}
}

func TestWithMaxDocs_Applies(t *testing.T) {
	cfg := defaultCfg()
	WithMaxDocs(10)(cfg)
	if cfg.maxDocs != 10 {
		t.Errorf("maxDocs: got %d want 10", cfg.maxDocs)
	}
}

func TestWithMaxDocs_ZeroIgnored(t *testing.T) {
	cfg := defaultCfg()
	// Zero value must not override the default.
	WithMaxDocs(0)(cfg)
	if cfg.maxDocs != defaultMaxDocs {
		t.Errorf("maxDocs after zero: got %d want default %d", cfg.maxDocs, defaultMaxDocs)
	}
}

func TestWithMaxCharsPerDoc_Applies(t *testing.T) {
	cfg := defaultCfg()
	WithMaxCharsPerDoc(256)(cfg)
	if cfg.maxCharsPerDoc != 256 {
		t.Errorf("maxCharsPerDoc: got %d want 256", cfg.maxCharsPerDoc)
	}
}

func TestWithObserver_Applies(t *testing.T) {
	cfg := defaultCfg()
	obs := &countingObserver{}
	WithObserver(obs)(cfg)
	if cfg.observer != obs {
		t.Error("observer not applied")
	}
}

func TestWithObserver_NilIgnored(t *testing.T) {
	cfg := defaultCfg()
	prev := cfg.observer
	WithObserver(nil)(cfg)
	if cfg.observer != prev {
		t.Error("nil observer must not replace existing observer")
	}
}

func TestWithHTTPClient_Applies(t *testing.T) {
	cfg := defaultCfg()
	custom := &http.Client{}
	WithHTTPClient(custom)(cfg)
	if cfg.hc != custom {
		t.Error("hc not applied")
	}
}

func TestWithHTTPClient_NilIgnored(t *testing.T) {
	cfg := defaultCfg()
	prev := cfg.hc
	WithHTTPClient(nil)(cfg)
	if cfg.hc != prev {
		t.Error("nil hc must not replace existing http.Client")
	}
}

func TestOpts_ChainApply(t *testing.T) {
	// Verify multiple Opts compose correctly on a single cfgInternal.
	cfg := defaultCfg()
	opts := []Opt{
		WithModel("m"),
		WithAPIKey("k"),
		WithTimeout(5 * time.Second),
		WithMaxDocs(20),
		WithMaxCharsPerDoc(512),
	}
	for _, opt := range opts {
		opt(cfg)
	}
	if cfg.model != "m" || cfg.apiKey != "k" || cfg.timeout != 5*time.Second ||
		cfg.maxDocs != 20 || cfg.maxCharsPerDoc != 512 {
		t.Errorf("chained opts: unexpected cfg %+v", cfg)
	}
}
