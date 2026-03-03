// Package hedge provides hedged request execution.
// It starts a primary call and, if it hasn't returned after a delay,
// launches a secondary (hedge) call. The first successful result wins.
package hedge

import (
	"context"
	"time"
)

type result[T any] struct {
	val T
	err error
}

// Do executes fn and, if it hasn't returned successfully after delay,
// launches fn again in parallel. The first success wins; if both fail,
// the primary error is returned.
//
// If delay <= 0, fn is called once directly without goroutines.
func Do[T any](ctx context.Context, delay time.Duration, fn func(context.Context) (T, error)) (T, error) {
	if delay <= 0 {
		return fn(ctx)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan result[T], 2)

	go func() {
		v, err := fn(ctx)
		ch <- result[T]{v, err}
	}()

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case r := <-ch:
		if r.err == nil {
			return r.val, nil
		}
		return r.val, r.err
	case <-timer.C:
		go func() {
			v, err := fn(ctx)
			ch <- result[T]{v, err}
		}()
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}

	// Two goroutines running — collect up to 2 results.
	var firstErr error
	for range 2 {
		select {
		case r := <-ch:
			if r.err == nil {
				return r.val, nil
			}
			if firstErr == nil {
				firstErr = r.err
			}
		case <-ctx.Done():
			var zero T
			if firstErr != nil {
				return zero, firstErr
			}
			return zero, ctx.Err()
		}
	}

	var zero T
	return zero, firstErr
}
