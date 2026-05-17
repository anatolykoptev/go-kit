// Package cmd provides a fluent text-command router for Telegram bots.
//
// # Usage
//
//	r := cmd.NewRouter()
//	r.On("/start", handleStart).Help("Запустить бота").Alias("/begin")
//	r.On("/domains", handleDomains).Help("Получить адрес для звонков")
//	r.OnDefault(handleUnknown)
//	r.AutoHelp("/help", func(ctx context.Context, upd *tgbotapi.Update) error {
//	    chatID := upd.Message.Chat.ID
//	    // send r.HelpText() via your preferred transport
//	    return nil
//	}).Help("Show available commands")
//
//	// Dispatch directly:
//	err := r.Dispatch(ctx, update)
//
//	// Or inspect and wrap:
//	handler, ok := r.Resolve(text)
//
// Router.Dispatch satisfies middleware.Handler and composes with middleware.Chain
// as the innermost handler.
//
// Concept: collapses bot_kit.go::handleCommand switch (oxpulse-admin/internal/bootstrap/handlers.go:91-130).
package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tgbotapi "github.com/OvyFlash/telegram-bot-api"
)

// Handler is the unit of work for a text command: process a Telegram update and return an error.
// It is intentionally identical to middleware.Handler so Router.Dispatch composes with
// middleware.Chain without an adapter.
type Handler func(ctx context.Context, upd *tgbotapi.Update) error

// Route is a single command registration returned by Router.On.
// Its fluent methods (.Help, .Alias, .When) configure the route inline.
type Route struct {
	primary    string
	handler    Handler
	helpText   string
	aliases    []string
	predicates []Predicate
	router     *Router
}

// Help sets the human-readable description shown in auto-generated /help output.
// Pass an empty string or omit the call to hide the command from /help.
func (rt *Route) Help(text string) *Route {
	rt.helpText = text
	return rt
}

// When adds a Predicate that must pass for this route to be selected.
// Multiple When calls compose as AND — all predicates must pass.
// Routes without any When calls always match (backward-compatible).
func (rt *Route) When(p Predicate) *Route {
	rt.predicates = append(rt.predicates, p)
	return rt
}

// Alias registers additional command strings that map to the same handler.
// Panics if any alias conflicts with an already-registered primary or alias.
func (rt *Route) Alias(cmds ...string) *Route {
	for _, a := range cmds {
		if _, exists := rt.router.index[a]; exists {
			panic(fmt.Sprintf("cmd.Router: alias %q conflicts with an existing registration", a))
		}
		rt.router.index[a] = rt
		rt.aliases = append(rt.aliases, a)
	}
	return rt
}

// Router dispatches Telegram text commands to registered handlers.
// Use NewRouter to create one; zero value is not ready.
type Router struct {
	routes  []*Route          // registration order preserved for HelpText sort
	index   map[string]*Route // primary + aliases
	def     Handler           // fallback for unrecognised commands
	helpCmd string            // set by AutoHelp
}

// NewRouter returns a ready-to-use Router.
func NewRouter() *Router {
	return &Router{index: make(map[string]*Route)}
}

// On registers cmd (e.g. "/start") with the given handler.
// Returns a *Route for fluent chaining (.Help, .Alias).
// Panics if cmd is already registered.
func (r *Router) On(cmd string, h Handler) *Route {
	if _, exists := r.index[cmd]; exists {
		panic(fmt.Sprintf("cmd.Router: command %q already registered", cmd))
	}
	rt := &Route{
		primary: cmd,
		handler: h,
		router:  r,
	}
	r.routes = append(r.routes, rt)
	r.index[cmd] = rt
	return rt
}

// OnDefault sets the fallback handler for unrecognised commands and non-command text.
func (r *Router) OnDefault(h Handler) {
	r.def = h
}

// AutoHelp registers cmd (e.g. "/help") with the given handler AND marks this
// command as the help command so r.HelpText() includes it in the listing.
//
// The handler is typically a small closure that calls r.HelpText() and sends
// the result via the caller's preferred transport — cmd.Router intentionally
// stays send-agnostic.
//
// Returns the Route for fluent chaining (.Help, .Alias).
// Panics if cmd is already registered (same rule as On).
//
// Typical wiring:
//
//	r.AutoHelp("/help", func(ctx context.Context, upd *tgbotapi.Update) error {
//	    chatID := upd.Message.Chat.ID
//	    return sender.Send(chatID, r.HelpText())
//	}).Help("Show available commands")
func (r *Router) AutoHelp(cmd string, h Handler) *Route {
	r.helpCmd = cmd
	return r.On(cmd, h)
}

// HelpText returns a formatted string listing all commands that have a Help() string,
// sorted lexicographically by primary command name.
// Aliases are listed inline after the primary command.
//
// Format per line: "/cmd[, /alias, ...] — description"
func (r *Router) HelpText() string {
	type entry struct {
		names string
		help  string
	}

	var entries []entry
	for _, rt := range r.routes {
		if rt.helpText == "" {
			continue // no help text — skip
		}
		names := rt.primary
		if len(rt.aliases) > 0 {
			names += ", " + strings.Join(rt.aliases, ", ")
		}
		entries = append(entries, entry{names: names, help: rt.helpText})
	}

	// Stable lexicographic order by primary command name.
	sort.Slice(entries, func(i, j int) bool {
		// Primary command is always the first token before the first comma.
		pi := strings.SplitN(entries[i].names, ",", 2)[0]
		pj := strings.SplitN(entries[j].names, ",", 2)[0]
		return pi < pj
	})

	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(e.names)
		sb.WriteString(" — ")
		sb.WriteString(e.help)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Resolve extracts the command token from text and looks up the registered handler.
//
// Extraction rules:
//   - Takes the first whitespace-separated token.
//   - Strips @botname suffix (e.g. "/start@my_bot" → "/start").
//   - Does not require a "/" prefix; non-command text routes to the default handler.
//
// Predicate-free lookup: predicates attached via When are NOT evaluated.
// Use ResolveFor when predicate evaluation is required.
//
// Returns (handler, true) if found (primary, alias, or default).
// Returns (nil, false) if not found and no default is registered.
func (r *Router) Resolve(text string) (Handler, bool) {
	token := extractCommand(text)

	if rt, ok := r.index[token]; ok {
		return rt.handler, true
	}
	if r.def != nil {
		return r.def, true
	}
	return nil, false
}

// ResolveFor extracts the command token from text, looks up the route, and
// evaluates any predicates attached via When against upd.
//
// If the route has no predicates the behaviour is identical to Resolve.
// If any predicate fails, the route is skipped and the default handler is
// returned (if registered), exactly as if the command were unrecognised.
//
// Returns (handler, true) on success.
// Returns (nil, false) if the command is unknown, predicates fail, and no default is set.
func (r *Router) ResolveFor(text string, upd *tgbotapi.Update) (Handler, bool) {
	token := extractCommand(text)

	if rt, ok := r.index[token]; ok {
		if r.matchPredicates(rt, upd) {
			return rt.handler, true
		}
		// Predicate failed — fall through to default.
		if r.def != nil {
			return r.def, true
		}
		return nil, false
	}
	if r.def != nil {
		return r.def, true
	}
	return nil, false
}

// matchPredicates returns true when all predicates on rt pass for upd.
// Routes with no predicates always match.
func (r *Router) matchPredicates(rt *Route, upd *tgbotapi.Update) bool {
	for _, p := range rt.predicates {
		if !p(upd) {
			return false
		}
	}
	return true
}

// Dispatch resolves the command in upd.Message.Text, evaluates route predicates,
// and calls the appropriate handler.
// It satisfies middleware.Handler and can be used as the innermost handler in a
// middleware.Chain.
//
// If upd.Message is nil, Dispatch returns nil (non-message updates are silently ignored).
// If no handler is found and no default is registered, Dispatch returns nil.
func (r *Router) Dispatch(ctx context.Context, upd *tgbotapi.Update) error {
	if upd.Message == nil {
		return nil
	}
	h, ok := r.ResolveFor(upd.Message.Text, upd)
	if !ok {
		return nil
	}
	return h(ctx, upd)
}

// extractCommand returns the command token from text:
//   - first whitespace-separated token
//   - @botname suffix stripped
func extractCommand(text string) string {
	token := strings.Fields(text)
	if len(token) == 0 {
		return ""
	}
	cmd := token[0]
	// Strip @botname suffix: "/start@my_bot" → "/start"
	if at := strings.IndexByte(cmd, '@'); at != -1 {
		cmd = cmd[:at]
	}
	return cmd
}
