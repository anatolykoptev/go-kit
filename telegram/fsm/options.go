package fsm

import "context"

// Option configures a Machine at construction time.
type Option func(*machineConfig)

// machineConfig holds optional Machine settings populated by Option functions.
type machineConfig struct {
	// cancelCmds is the set of text commands that trigger session cancellation.
	cancelCmds map[string]struct{}
	// onCancel is called (if non-nil) after a session is cancelled via a cancel command.
	onCancel func(ctx context.Context, chatID int64)
}

// WithCancelCmds registers one or more text commands (e.g. "/cancel", "stop")
// that, when received during an active session, immediately cancel the session
// without invoking the current StateFn.
//
// If the received message matches any registered command, Feed returns
// (handled=true, nil) after deleting the session and invoking the OnCancel
// hook (if configured).
func WithCancelCmds(cmds ...string) Option {
	return func(cfg *machineConfig) {
		if cfg.cancelCmds == nil {
			cfg.cancelCmds = make(map[string]struct{}, len(cmds))
		}
		for _, cmd := range cmds {
			cfg.cancelCmds[cmd] = struct{}{}
		}
	}
}

// WithOnCancel sets a hook that is invoked whenever a session is terminated
// by a cancel command (see WithCancelCmds). The hook receives the context
// passed to Feed and the chatID of the cancelled session.
//
// The hook is called synchronously inside Feed before returning.
func WithOnCancel(fn func(ctx context.Context, chatID int64)) Option {
	return func(cfg *machineConfig) {
		cfg.onCancel = fn
	}
}
