package skills

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// recordingTracer — captures span events for assertions
// ---------------------------------------------------------------------------

type recordedSpan struct {
	name  string
	label string // source for body, outcome for catalog
}

type recordingTracer struct {
	mu      sync.Mutex
	body    []recordedSpan
	catalog []recordedSpan

	// capturedCtx stores the ctx passed to startBody for context-probe tests.
	capturedCtx context.Context //nolint:containedctx // intentional for test capture
}

func (r *recordingTracer) startBody(ctx context.Context, name string) (context.Context, func(string)) {
	r.mu.Lock()
	r.capturedCtx = ctx
	r.mu.Unlock()
	return ctx, func(src string) {
		r.mu.Lock()
		r.body = append(r.body, recordedSpan{name, src})
		r.mu.Unlock()
	}
}

func (r *recordingTracer) startCatalogLoad(_ context.Context, name string) (context.Context, func(string)) {
	return context.Background(), func(outcome string) {
		r.mu.Lock()
		r.catalog = append(r.catalog, recordedSpan{name, outcome})
		r.mu.Unlock()
	}
}

func (r *recordingTracer) tracer() *Tracer {
	return &Tracer{
		StartBody:        r.startBody,
		StartCatalogLoad: r.startCatalogLoad,
	}
}

// ---------------------------------------------------------------------------
// env helpers reused from observer_test (same package)
// ---------------------------------------------------------------------------

const tracerEnvKey = "SKILLS_TRACER_TEST_PATH"

const sampleTracerRaw = "---\nname: t10\ndescription: Tracer test skill.\n---\n\nTracer test body."

// newTracerEmbedded creates an Embedded configured with the given options.
func newTracerEmbedded(opts ...EmbeddedOption) *Embedded {
	return NewEmbedded("t10", tracerEnvKey, sampleTracerRaw, opts...)
}

// setTracerEnv sets tracerEnvKey and restores on cleanup.
func setTracerEnv(t *testing.T, val string) {
	t.Helper()
	prev, had := os.LookupEnv(tracerEnvKey)
	if err := os.Setenv(tracerEnvKey, val); err != nil {
		t.Fatalf("setTracerEnv: %v", err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv(tracerEnvKey, prev) //nolint:errcheck
		} else {
			os.Unsetenv(tracerEnvKey) //nolint:errcheck
		}
	})
}

// unsetTracerEnv clears tracerEnvKey and restores on cleanup.
func unsetTracerEnv(t *testing.T) {
	t.Helper()
	prev, had := os.LookupEnv(tracerEnvKey)
	os.Unsetenv(tracerEnvKey) //nolint:errcheck
	t.Cleanup(func() {
		if had {
			os.Setenv(tracerEnvKey, prev) //nolint:errcheck
		}
	})
}

// writeTracerSkill writes content to a temp file and returns its path.
func writeTracerSkill(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTracerSkill: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// Embedded.BodyCtx tests
// ---------------------------------------------------------------------------

func TestEmbedded_BodyCtx_NilTracer_BehavesLikeBody(t *testing.T) {
	unsetTracerEnv(t)
	e := newTracerEmbedded() // no tracer

	body := e.Body()
	bodyCtx := e.BodyCtx(context.Background())

	if body != bodyCtx {
		t.Errorf("BodyCtx = %q, want %q (same as Body)", bodyCtx, body)
	}
	if body == "" {
		t.Fatal("expected non-empty body")
	}
}

func TestEmbedded_BodyCtx_FiresStartBody(t *testing.T) {
	unsetTracerEnv(t)
	rec := &recordingTracer{}
	e := newTracerEmbedded(WithTracer(rec.tracer()))

	_ = e.BodyCtx(context.Background())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.body) != 1 {
		t.Fatalf("body spans = %d, want 1; spans = %v", len(rec.body), rec.body)
	}
	if rec.body[0].name != "t10" {
		t.Errorf("span name = %q, want t10", rec.body[0].name)
	}
}

func TestEmbedded_BodyCtx_SourceLabel_Embedded(t *testing.T) {
	unsetTracerEnv(t)
	rec := &recordingTracer{}
	e := newTracerEmbedded(WithTracer(rec.tracer()))

	_ = e.BodyCtx(context.Background())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.body) != 1 || rec.body[0].label != "embedded" {
		t.Errorf("body spans = %v, want [{t10 embedded}]", rec.body)
	}
}

func TestEmbedded_BodyCtx_SourceLabel_Env(t *testing.T) {
	content := "---\nname: t10\ndescription: Override.\n---\n\nEnv body content."
	path := writeTracerSkill(t, content)
	setTracerEnv(t, path)

	rec := &recordingTracer{}
	e := newTracerEmbedded(WithTracer(rec.tracer()))

	_ = e.BodyCtx(context.Background())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.body) != 1 || rec.body[0].label != "env" {
		t.Errorf("body spans = %v, want [{t10 env}]", rec.body)
	}
}

func TestEmbedded_BodyCtx_SourceLabel_CacheHit(t *testing.T) {
	content := "---\nname: t10\ndescription: Override.\n---\n\nCached body."
	path := writeTracerSkill(t, content)
	setTracerEnv(t, path)

	rec := &recordingTracer{}
	e := newTracerEmbedded(WithTracer(rec.tracer()))

	// First call: env read.
	_ = e.BodyCtx(context.Background())
	// Second call: same mtime → cache_hit.
	_ = e.BodyCtx(context.Background())

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.body) != 2 { //nolint:mnd
		t.Fatalf("body span count = %d, want 2; spans = %v", len(rec.body), rec.body)
	}
	if rec.body[0].label != "env" {
		t.Errorf("first span label = %q, want env", rec.body[0].label)
	}
	if rec.body[1].label != "cache_hit" {
		t.Errorf("second span label = %q, want cache_hit", rec.body[1].label)
	}
}

func TestEmbedded_BodyCtx_NilStartBody_NoOp(t *testing.T) {
	unsetTracerEnv(t)
	// Tracer set but StartBody field nil — must behave like Body().
	tr := &Tracer{StartBody: nil}
	e := newTracerEmbedded(WithTracer(tr))

	body := e.Body()
	bodyCtx := e.BodyCtx(context.Background())

	if body != bodyCtx {
		t.Errorf("BodyCtx = %q, want %q (same as Body)", bodyCtx, body)
	}
}

func TestEmbedded_BodyCtx_PreservesContext(t *testing.T) {
	unsetTracerEnv(t)

	type ctxKey struct{}
	sentinel := "tracer-probe-value"

	var capturedCtx context.Context
	tr := &Tracer{
		StartBody: func(ctx context.Context, _ string) (context.Context, func(string)) {
			capturedCtx = ctx
			return ctx, func(string) {}
		},
	}
	e := newTracerEmbedded(WithTracer(tr))

	ctx := context.WithValue(context.Background(), ctxKey{}, sentinel)
	_ = e.BodyCtx(ctx)

	if capturedCtx == nil {
		t.Fatal("StartBody was not called")
	}
	got, ok := capturedCtx.Value(ctxKey{}).(string)
	if !ok || got != sentinel {
		t.Errorf("capturedCtx value = %q %v, want %q", got, ok, sentinel)
	}
}

// ---------------------------------------------------------------------------
// Catalog.WithTracer / LoadCtx tracer tests
// ---------------------------------------------------------------------------

func TestCatalog_LoadCtx_FiresStartCatalogLoad(t *testing.T) {
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	rec := &recordingTracer{}
	cat = cat.WithTracer(rec.tracer())

	_, _, _, err := cat.LoadCtx(t.Context(), "foo")
	if err != nil {
		t.Fatalf("LoadCtx: %v", err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.catalog) != 1 {
		t.Fatalf("catalog spans = %d, want 1; spans = %v", len(rec.catalog), rec.catalog)
	}
	if rec.catalog[0].name != "foo" {
		t.Errorf("span name = %q, want foo", rec.catalog[0].name)
	}
}

func TestCatalog_LoadCtx_OutcomeHit(t *testing.T) {
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	rec := &recordingTracer{}
	cat = cat.WithTracer(rec.tracer())

	_, _, ok, err := cat.LoadCtx(t.Context(), "foo")
	if err != nil || !ok {
		t.Fatalf("LoadCtx: err=%v ok=%v", err, ok)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.catalog) != 1 || rec.catalog[0].label != "hit" {
		t.Errorf("catalog spans = %v, want [{foo hit}]", rec.catalog)
	}
}

func TestCatalog_LoadCtx_OutcomeMiss(t *testing.T) {
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	rec := &recordingTracer{}
	cat = cat.WithTracer(rec.tracer())

	_, _, ok, err := cat.LoadCtx(t.Context(), "nonexistent-skill-xyz")
	if err != nil || ok {
		t.Fatalf("LoadCtx: err=%v ok=%v (want ok=false)", err, ok)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.catalog) != 1 || rec.catalog[0].label != "miss" {
		t.Errorf("catalog spans = %v, want [{nonexistent-skill-xyz miss}]", rec.catalog)
	}
}

func TestCatalog_LoadCtx_NilTracer_StillWorks(t *testing.T) {
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	// No tracer attached — LoadCtx must work normally.
	body, _, ok, err := cat.LoadCtx(t.Context(), "foo")
	if err != nil || !ok {
		t.Fatalf("LoadCtx: err=%v ok=%v", err, ok)
	}
	if body == "" {
		t.Fatal("expected non-empty body")
	}
}

func TestCatalog_LoadCtx_NilStartCatalogLoad_NoOp(t *testing.T) {
	cat := NewCatalog(NewDirTier("builtin", filepath.Join("testdata/tier", "builtin")))
	// Tracer set but StartCatalogLoad field nil — must behave normally.
	tr := &Tracer{StartCatalogLoad: nil}
	cat = cat.WithTracer(tr)

	body, _, ok, err := cat.LoadCtx(t.Context(), "foo")
	if err != nil || !ok {
		t.Fatalf("LoadCtx: err=%v ok=%v", err, ok)
	}
	if body == "" {
		t.Fatal("expected non-empty body")
	}
}
