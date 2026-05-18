package llm

import "context"

// NoOp is a Completer that always returns ErrUnavailable. It exists so
// callers can wire a non-nil Completer when no API key is configured,
// avoiding nil-checks at every call site.
type NoOp struct{}

func (NoOp) Complete(context.Context, string, string, ...ChatOption) (string, error) {
	return "", ErrUnavailable
}

// NewOptional returns a real *Client when apiKey is non-empty, otherwise
// NoOp{}. The bool reports whether a real client was constructed — useful
// for startup logging and metric labels. NewOptional never returns nil.
func NewOptional(baseURL, apiKey, model string, opts ...Option) (Completer, bool) {
	if apiKey == "" {
		return NoOp{}, false
	}
	return NewClient(baseURL, apiKey, model, opts...), true
}
