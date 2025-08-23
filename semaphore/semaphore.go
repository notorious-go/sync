package semaphore

import (
	"fmt"
)

// Semaphore is a counting semaphore implemented as a buffered channel, wherein
// the buffer size determines the maximum number of concurrent operations.
//
// The nil Semaphore is the zero-value Semaphore. It represents unlimited
// capacity and never blocks.
//
// To inspect the semaphore's current state, use the built-in len and cap functions:
//   - len(s) returns the number of tokens currently acquired but not yet released.
//   - cap(s) returns the maximum number of tokens that can be acquired at once.
//   - cap(s) - len(s) gives the number of available slots.
//
// For nil semaphores, both len and cap return 0.
type Semaphore chan struct{}

// New creates semaphores with the specified limit. A negative limit
// indicates an unlimited semaphore that never blocks on acquisition.
//
// This design choice allows callers to use the same interface whether they want
// bounded or unbounded concurrency without special-casing in their code.
func New(limit int) Semaphore {
	if limit < 0 {
		// The nil Semaphore has no limit, which is the meaning of setting the limit
		// parameter to negative values.
		return nil
	}
	return make(Semaphore, limit)
}

// String returns a human-readable representation of the semaphore's state.
// For bounded semaphores, it shows "Semaphore(acquired/capacity)" format.
// For nil (unlimited) semaphores, it returns "Semaphore(unlimited)".
// This method enables direct printing of semaphores in fmt operations.
func (s Semaphore) String() string {
	if s == nil {
		return "Semaphore(unlimited)"
	}
	return fmt.Sprintf("Semaphore(%v/%v)", len(s), cap(s))
}

// Acquire blocks until a token becomes available, then acquires it.
//
// For nil semaphores (unlimited capacity), this never blocks and returns
// immediately. For bounded semaphores, this will block if all tokens are
// currently held by other goroutines.
//
// Typical usage pattern:
//
//	s.Acquire()
//	defer s.Release()
//	// ... do work ...
func (s Semaphore) Acquire() {
	if s == nil {
		// The nil Semaphore has no limit, which means it allows acquiring another token
		// immediately.
		return
	}
	s <- struct{}{}
}

// Release returns a token to the semaphore, allowing another blocked goroutine
// to proceed. Release must be called exactly once for each successful Acquire or
// TryAcquire(true) to maintain the semaphore's count invariant.
//
// For nil semaphores, this is a no-op since tokens were never actually limited.
//
// Calling Release more times than Acquire will block because it will attempt to
// send to a channel that has no receivers.
func (s Semaphore) Release() {
	if s == nil {
		// The nil Semaphore has no limit, which means it does not require actually
		// releasing a token because it was never actually acquired in the first place.
		return
	}
	<-s
}

// TryAcquire attempts to acquire a token without blocking. Returns true if a
// token was acquired, false if the semaphore is at capacity.
//
// For nil semaphores, it always returns true since capacity is unlimited.
//
// Note: TryAcquire may succeed even when other goroutines are blocked on
// Acquire, as it inherits Go's channel "barging" behaviour. This means there is
// no strict FIFO ordering between blocking Acquire calls and non-blocking
// TryAcquire attempts.
//
// This is useful when you want to attempt an operation but fall back to
// alternative behaviour if the semaphore is full:
//
//	if s.TryAcquire() {
//	    defer s.Release()
//	    // ... do work ...
//	} else {
//	    // ... handle the "too busy" case ...
//	}
func (s Semaphore) TryAcquire() bool {
	if s == nil {
		// The nil Semaphore has no limit, which means it does not block and always
		// allows acquiring another token.
		return true
	}
	select {
	case s <- struct{}{}:
		return true
	default:
		return false
	}
}
