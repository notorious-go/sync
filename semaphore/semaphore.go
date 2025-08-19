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

// NewSemaphore creates semaphores with the specified limit. A negative limit
// indicates an unlimited semaphore that never blocks on acquisition.
//
// This design choice allows callers to use the same interface whether they want
// bounded or unbounded concurrency without special-casing in their code.
func NewSemaphore(limit int) Semaphore {
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
