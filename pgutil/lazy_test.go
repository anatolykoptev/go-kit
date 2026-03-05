package pgutil

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeConn struct{ id int }

// fastTicker returns a tickerFn that sends on a channel controlled by the test.
func fastTicker() (func(time.Duration) ticker, chan time.Time) {
	ch := make(chan time.Time, 1)
	fn := func(_ time.Duration) ticker {
		return ticker{C: ch, stop: func() {}}
	}
	return fn, ch
}

func newTestLazy(
	connectFn func(ctx context.Context) (*fakeConn, error),
	closeFn func(*fakeConn),
	healthFn func(ctx context.Context, conn *fakeConn) error,
) (*Lazy[*fakeConn], chan time.Time) {
	tickFn, tickCh := fastTicker()
	l := NewLazy(
		Options{MaxAttempts: 2, InitDelay: time.Millisecond, MaxDelay: time.Millisecond},
		connectFn, closeFn, healthFn,
	)
	l.tickerFn = tickFn
	return l, tickCh
}

func TestLazy_GetBeforeStart(t *testing.T) {
	l, _ := newTestLazy(
		func(_ context.Context) (*fakeConn, error) { return &fakeConn{1}, nil },
		func(_ *fakeConn) {},
		func(_ context.Context, _ *fakeConn) error { return nil },
	)
	if l.Get() != nil {
		t.Fatal("expected nil before Start")
	}
	if l.Ready() {
		t.Fatal("expected not ready before Start")
	}
}

func TestLazy_ConnectSuccess(t *testing.T) {
	l, _ := newTestLazy(
		func(_ context.Context) (*fakeConn, error) { return &fakeConn{id: 42}, nil },
		func(_ *fakeConn) {},
		func(_ context.Context, _ *fakeConn) error { return nil },
	)

	l.Start(context.Background())
	defer l.Close()

	// Wait for connection.
	deadline := time.After(time.Second)
	for !l.Ready() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for Ready")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	conn := l.Get()
	if conn == nil || conn.id != 42 {
		t.Fatalf("expected conn.id=42, got %v", conn)
	}
}

func TestLazy_ConnectRetryThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	l, _ := newTestLazy(
		func(_ context.Context) (*fakeConn, error) {
			n := attempts.Add(1)
			if n < 3 {
				return nil, errors.New("refused")
			}
			return &fakeConn{id: 1}, nil
		},
		func(_ *fakeConn) {},
		func(_ context.Context, _ *fakeConn) error { return nil },
	)

	l.Start(context.Background())
	defer l.Close()

	deadline := time.After(2 * time.Second)
	for !l.Ready() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for Ready")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	if l.Get() == nil {
		t.Fatal("expected non-nil conn")
	}
	if n := attempts.Load(); n < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", n)
	}
}

func TestLazy_HealthFailTriggersReconnect(t *testing.T) {
	var connectCount atomic.Int32
	var closed atomic.Int32
	healthFailOnce := make(chan struct{}, 1)
	healthFailOnce <- struct{}{} // one failure loaded

	l, tickCh := newTestLazy(
		func(_ context.Context) (*fakeConn, error) {
			n := connectCount.Add(1)
			return &fakeConn{id: int(n)}, nil
		},
		func(_ *fakeConn) { closed.Add(1) },
		func(_ context.Context, _ *fakeConn) error {
			select {
			case <-healthFailOnce:
				return errors.New("unhealthy")
			default:
				return nil
			}
		},
	)

	l.Start(context.Background())
	defer l.Close()

	// Wait for first connect.
	waitReady(t, l, true, "first connect")

	if connectCount.Load() != 1 {
		t.Fatalf("expected 1 connect, got %d", connectCount.Load())
	}

	// Trigger health fail → reconnect cycle.
	tickCh <- time.Now()

	// Wait for second connect (reconnect after health failure).
	deadline := time.After(time.Second)
	for connectCount.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for reconnect")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	waitReady(t, l, true, "reconnect")

	if closed.Load() < 1 {
		t.Fatal("expected closeFn to be called on health failure")
	}
	if l.Get().id != int(connectCount.Load()) {
		t.Fatal("expected new conn after reconnect")
	}
}

func waitReady(t *testing.T, l *Lazy[*fakeConn], want bool, label string) {
	t.Helper()
	deadline := time.After(time.Second)
	for l.Ready() != want {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for Ready=%v (%s)", want, label)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestLazy_CloseStopsLoop(t *testing.T) {
	var closed atomic.Bool
	l, _ := newTestLazy(
		func(_ context.Context) (*fakeConn, error) { return &fakeConn{id: 1}, nil },
		func(_ *fakeConn) { closed.Store(true) },
		func(_ context.Context, _ *fakeConn) error { return nil },
	)

	l.Start(context.Background())

	deadline := time.After(time.Second)
	for !l.Ready() {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for Ready")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	l.Close()

	if !closed.Load() {
		t.Fatal("expected closeFn called on Close")
	}
	if l.Ready() {
		t.Fatal("expected not ready after Close")
	}
}

func TestLazy_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	l, _ := newTestLazy(
		func(_ context.Context) (*fakeConn, error) {
			return nil, errors.New("refused")
		},
		func(_ *fakeConn) {},
		func(_ context.Context, _ *fakeConn) error { return nil },
	)

	l.Start(ctx)
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Close should not hang.
	done := make(chan struct{})
	go func() {
		l.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close hung after context cancel")
	}
}
