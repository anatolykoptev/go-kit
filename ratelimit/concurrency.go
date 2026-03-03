package ratelimit

import "context"

// ConcurrencyLimiter limits concurrent execution using a buffered channel as semaphore.
// Send = acquire, receive = release. Goroutine-safe.
type ConcurrencyLimiter struct {
	sem  chan struct{}
	size int
}

// NewConcurrencyLimiter creates a limiter allowing at most maxConcurrent
// simultaneous operations. Panics if maxConcurrent <= 0.
func NewConcurrencyLimiter(maxConcurrent int) *ConcurrencyLimiter {
	if maxConcurrent <= 0 {
		panic("ratelimit: maxConcurrent must be positive")
	}
	return &ConcurrencyLimiter{
		sem:  make(chan struct{}, maxConcurrent),
		size: maxConcurrent,
	}
}

// Acquire blocks until a slot is available or ctx is cancelled.
// Returns a release function and nil error on success.
// The caller must call the release function when done.
func (cl *ConcurrencyLimiter) Acquire(ctx context.Context) (func(), error) {
	select {
	case cl.sem <- struct{}{}:
		return cl.release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// TryAcquire attempts to acquire a slot without blocking.
// Returns a release function and true if successful, nil and false otherwise.
func (cl *ConcurrencyLimiter) TryAcquire() (func(), bool) {
	select {
	case cl.sem <- struct{}{}:
		return cl.release, true
	default:
		return nil, false
	}
}

// release returns one slot to the semaphore.
func (cl *ConcurrencyLimiter) release() {
	<-cl.sem
}

// Available returns the number of currently available slots.
func (cl *ConcurrencyLimiter) Available() int {
	return cl.size - len(cl.sem)
}

// Size returns the maximum number of concurrent slots.
func (cl *ConcurrencyLimiter) Size() int {
	return cl.size
}
