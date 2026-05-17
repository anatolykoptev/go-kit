package cmd_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/anatolykoptev/go-kit/telegram/cmd"
	mw "github.com/anatolykoptev/go-kit/telegram/middleware"
)

// makeUpdate builds an Update with Message.Text set.
func makeUpdate(text string) *tgbotapi.Update {
	return &tgbotapi.Update{
		Message: &tgbotapi.Message{Text: text},
	}
}

// noopHandler is a cmd.Handler that records invocation.
func noopHandler(called *bool) cmd.Handler {
	return func(_ context.Context, _ *tgbotapi.Update) error {
		*called = true
		return nil
	}
}

// errHandler always returns a sentinel error.
func errHandler(err error) cmd.Handler {
	return func(_ context.Context, _ *tgbotapi.Update) error { return err }
}

// ─── Resolve ────────────────────────────────────────────────────────────────

// TestResolve_exactCommand verifies that a registered command resolves to its handler.
func TestResolve_exactCommand(t *testing.T) {
	var called bool
	r := cmd.NewRouter()
	r.On("/start", noopHandler(&called))

	h, ok := r.Resolve("/start")
	if !ok {
		t.Fatal("expected Resolve to return ok=true for registered command")
	}
	if err := h(context.Background(), makeUpdate("/start")); err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
}

// TestResolve_unknownNoDefault verifies that an unregistered command without OnDefault returns (nil, false).
func TestResolve_unknownNoDefault(t *testing.T) {
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil })

	h, ok := r.Resolve("/unknown")
	if ok {
		t.Fatal("expected ok=false for unknown command with no default")
	}
	if h != nil {
		t.Fatal("expected nil handler for unknown command with no default")
	}
}

// TestResolve_unknownWithDefault verifies that OnDefault handler is returned for unrecognised commands.
func TestResolve_unknownWithDefault(t *testing.T) {
	var called bool
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil })
	r.OnDefault(noopHandler(&called))

	h, ok := r.Resolve("/unknown")
	if !ok {
		t.Fatal("expected ok=true when default handler is registered")
	}
	if err := h(context.Background(), makeUpdate("/unknown")); err != nil {
		t.Fatalf("default handler returned unexpected error: %v", err)
	}
	if !called {
		t.Fatal("default handler was not called")
	}
}

// TestResolve_nonCommandTextWithDefault verifies non-"/" text routes to default.
func TestResolve_nonCommandTextWithDefault(t *testing.T) {
	var called bool
	r := cmd.NewRouter()
	r.OnDefault(noopHandler(&called))

	h, ok := r.Resolve("hello world")
	if !ok {
		t.Fatal("expected ok=true for non-command text when default handler registered")
	}
	_ = h(context.Background(), makeUpdate("hello world"))
	if !called {
		t.Fatal("default handler was not called for non-command text")
	}
}

// TestResolve_nonCommandTextNoDefault verifies non-"/" text with no default → (nil, false).
func TestResolve_nonCommandTextNoDefault(t *testing.T) {
	r := cmd.NewRouter()
	h, ok := r.Resolve("hello world")
	if ok {
		t.Fatal("expected ok=false for non-command text with no default")
	}
	if h != nil {
		t.Fatal("expected nil handler for non-command text with no default")
	}
}

// TestResolve_botnameSuffix verifies /start@botname routes to /start handler.
func TestResolve_botnameSuffix(t *testing.T) {
	var called bool
	r := cmd.NewRouter()
	r.On("/start", noopHandler(&called))

	h, ok := r.Resolve("/start@my_bot")
	if !ok {
		t.Fatal("expected ok=true for command with @botname suffix")
	}
	_ = h(context.Background(), makeUpdate("/start@my_bot"))
	if !called {
		t.Fatal("handler was not called for /start@my_bot")
	}
}

// TestResolve_onlyFirstToken verifies that only the first token is matched.
func TestResolve_onlyFirstToken(t *testing.T) {
	var called bool
	r := cmd.NewRouter()
	r.On("/start", noopHandler(&called))

	h, ok := r.Resolve("/start some args")
	if !ok {
		t.Fatal("expected ok=true for command with trailing args")
	}
	_ = h(context.Background(), makeUpdate("/start some args"))
	if !called {
		t.Fatal("handler not called when command has trailing args")
	}
}

// ─── Alias ──────────────────────────────────────────────────────────────────

// TestAlias_resolvesAlias verifies that an alias maps to the same handler.
func TestAlias_resolvesAlias(t *testing.T) {
	var count int
	h := func(_ context.Context, _ *tgbotapi.Update) error { count++; return nil }
	r := cmd.NewRouter()
	r.On("/start", h).Alias("/begin", "/go")

	for _, alias := range []string{"/start", "/begin", "/go"} {
		rh, ok := r.Resolve(alias)
		if !ok {
			t.Fatalf("expected ok=true for %q", alias)
		}
		_ = rh(context.Background(), makeUpdate(alias))
	}
	if count != 3 {
		t.Fatalf("expected handler called 3 times, got %d", count)
	}
}

// ─── Dispatch ───────────────────────────────────────────────────────────────

// TestDispatch_basic verifies that Dispatch routes correctly and returns handler error.
func TestDispatch_basic(t *testing.T) {
	sentinel := errors.New("sentinel")
	r := cmd.NewRouter()
	r.On("/start", errHandler(sentinel))

	upd := makeUpdate("/start")
	err := r.Dispatch(context.Background(), upd)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

// TestDispatch_noHandlerNoDefault returns nil when no default registered.
func TestDispatch_noHandlerNoDefault(t *testing.T) {
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil })

	err := r.Dispatch(context.Background(), makeUpdate("/unknown"))
	if err != nil {
		t.Fatalf("expected nil from Dispatch with no default, got %v", err)
	}
}

// TestDispatch_nilMessage verifies that Dispatch handles nil Message gracefully.
func TestDispatch_nilMessage(t *testing.T) {
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil })

	err := r.Dispatch(context.Background(), &tgbotapi.Update{})
	if err != nil {
		t.Fatalf("expected nil for update with nil Message, got %v", err)
	}
}

// TestDispatch_middlewareChain verifies Router.Dispatch composes with middleware.Chain as innermost handler.
// This validates the spec claim: "Router.Dispatch is itself a middleware.Handler".
func TestDispatch_middlewareChain(t *testing.T) {
	var order []string

	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error {
		order = append(order, "handler")
		return nil
	})

	// Wrap with a middleware that records before/after.
	traceMW := func(next mw.Handler) mw.Handler {
		return func(ctx context.Context, upd *tgbotapi.Update) error {
			order = append(order, "before")
			err := next(ctx, upd)
			order = append(order, "after")
			return err
		}
	}

	chain := mw.Chain(traceMW)(r.Dispatch)
	err := chain(context.Background(), makeUpdate("/start"))
	if err != nil {
		t.Fatalf("chain returned unexpected error: %v", err)
	}
	want := []string{"before", "handler", "after"}
	if fmt.Sprintf("%v", order) != fmt.Sprintf("%v", want) {
		t.Fatalf("unexpected order: got %v, want %v", order, want)
	}
}

// ─── AutoHelp ───────────────────────────────────────────────────────────────

// TestAutoHelp_generated verifies that AutoHelp registers a /help command.
func TestAutoHelp_generated(t *testing.T) {
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Help("Start the bot")
	r.On("/domains", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Help("Get domains")
	r.AutoHelp("/help", func(_ context.Context, _ *tgbotapi.Update) error { return nil })

	h, ok := r.Resolve("/help")
	if !ok {
		t.Fatal("expected /help to be resolvable after AutoHelp")
	}

	// Call the help handler — it should work without error.
	err := h(context.Background(), makeUpdate("/help"))
	if err != nil {
		t.Fatalf("auto-help handler returned unexpected error: %v", err)
	}
}

// TestAutoHelp_stableOrder verifies that help text lists commands in lexicographic order.
func TestAutoHelp_stableOrder(t *testing.T) {
	r := cmd.NewRouter()
	r.On("/zzz", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Help("Last command")
	r.On("/aaa", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Help("First command")
	r.On("/mmm", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Help("Middle command")
	r.AutoHelp("/help", func(_ context.Context, _ *tgbotapi.Update) error { return nil })

	text := r.HelpText()
	// Commands should appear in lexicographic order.
	aIdx := strings.Index(text, "/aaa")
	mIdx := strings.Index(text, "/mmm")
	zIdx := strings.Index(text, "/zzz")
	if aIdx == -1 || mIdx == -1 || zIdx == -1 {
		t.Fatalf("not all commands found in help text: %q", text)
	}
	if !(aIdx < mIdx && mIdx < zIdx) {
		t.Fatalf("commands not in lexicographic order in help text: %q", text)
	}
}

// TestAutoHelp_textContainsCommandAndHelp verifies help text format.
func TestAutoHelp_textContainsCommandAndHelp(t *testing.T) {
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Help("Запустить бота")
	r.AutoHelp("/help", func(_ context.Context, _ *tgbotapi.Update) error { return nil })

	text := r.HelpText()
	if !strings.Contains(text, "/start") {
		t.Errorf("help text missing /start: %q", text)
	}
	if !strings.Contains(text, "Запустить бота") {
		t.Errorf("help text missing help string: %q", text)
	}
}

// TestAutoHelp_aliasInHelpText verifies aliases are listed in help text.
func TestAutoHelp_aliasInHelpText(t *testing.T) {
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Help("Запустить").Alias("/begin")
	r.AutoHelp("/help", func(_ context.Context, _ *tgbotapi.Update) error { return nil })

	text := r.HelpText()
	if !strings.Contains(text, "/begin") {
		t.Errorf("expected alias /begin in help text: %q", text)
	}
}

// TestAutoHelp_commandWithoutHelp verifies commands without Help() string are omitted from help.
func TestAutoHelp_commandWithoutHelp(t *testing.T) {
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Help("Описание")
	r.On("/hidden", func(_ context.Context, _ *tgbotapi.Update) error { return nil }) // no Help
	r.AutoHelp("/help", func(_ context.Context, _ *tgbotapi.Update) error { return nil })

	text := r.HelpText()
	if strings.Contains(text, "/hidden") {
		t.Errorf("expected /hidden (no help) to be absent from help text: %q", text)
	}
}

// ─── Conflicting registrations ───────────────────────────────────────────────

// TestConflict_duplicatePrimaryPanics verifies that registering the same primary command twice panics.
func TestConflict_duplicatePrimaryPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate primary command registration")
		}
	}()
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil })
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil }) // panic here
}

// TestConflict_aliasShadowsPrimaryPanics verifies that registering an alias that matches a primary panics.
func TestConflict_aliasShadowsPrimaryPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when alias shadows existing primary command")
		}
	}()
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil })
	r.On("/other", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Alias("/start") // panic here
}

// TestAutoHelp_HandlerExecuted verifies that AutoHelp registers the provided handler
// and it executes on dispatch (not a silent no-op sentinel).
func TestAutoHelp_HandlerExecuted(t *testing.T) {
	var called bool
	r := cmd.NewRouter()
	r.AutoHelp("/help", func(_ context.Context, upd *tgbotapi.Update) error {
		called = true
		return nil
	})

	err := r.Dispatch(context.Background(), makeUpdate("/help"))
	if err != nil {
		t.Fatalf("AutoHelp handler returned unexpected error: %v", err)
	}
	if !called {
		t.Fatal("AutoHelp handler was not called on dispatch")
	}
}

// TestAutoHelp_DuplicateRegistration_Panics verifies that calling AutoHelp then On
// with the same command panics (same rule as On+On).
func TestAutoHelp_DuplicateRegistration_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when registering same command after AutoHelp")
		}
	}()
	r := cmd.NewRouter()
	r.AutoHelp("/help", func(_ context.Context, _ *tgbotapi.Update) error { return nil })
	r.On("/help", func(_ context.Context, _ *tgbotapi.Update) error { return nil }) // must panic
}

// TestAutoHelp_Route_ChainsHelp verifies that AutoHelp returns a *Route so callers
// can chain .Help("...") and see /help included in HelpText output.
func TestAutoHelp_Route_ChainsHelp(t *testing.T) {
	r := cmd.NewRouter()
	r.On("/start", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Help("Start")
	r.AutoHelp("/help", func(_ context.Context, _ *tgbotapi.Update) error { return nil }).Help("Show help")

	text := r.HelpText()
	if !strings.Contains(text, "/help") {
		t.Errorf("expected /help in HelpText when chained with .Help(): %q", text)
	}
	if !strings.Contains(text, "Show help") {
		t.Errorf("expected 'Show help' in HelpText: %q", text)
	}
}
