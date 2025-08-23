package semaphore_test

import (
	"context"
	"fmt"

	"github.com/notorious-go/sync/semaphore"
)

func Example() {
	sem := semaphore.New(2)
	fmt.Println("Created:", sem)
	printSemaphore(sem, "Initial state")
	defer printSemaphore(sem, "Final state - all tokens released")

	// You should always pair Acquire with a deferred Release to ensure
	// tokens are returned even if your code panics.
	sem.Acquire()
	defer sem.Release()
	printSemaphore(sem, "After acquiring first token")

	// TryAcquire lets you handle the "too busy" case gracefully.
	// Here you'll succeed because the semaphore has room for 2 concurrent operations.
	if sem.TryAcquire() {
		defer sem.Release() // Always release eventually.
		printSemaphore(sem, "After acquiring second token")
	}

	// When at capacity, TryAcquire returns false immediately rather than blocking.
	// This lets you implement fallback logic or report back-pressure to users.
	if !sem.TryAcquire() {
		printSemaphore(sem, "TryAcquire failed - semaphore full")
		// This demonstrates that tokens are fungible - you can release
		// one token and acquire another. The semaphore doesn't track "who"
		// owns tokens, just the count.
		sem.Release()
		printSemaphore(sem, "After releasing one token")
		sem.Acquire()
		printSemaphore(sem, "After re-acquiring token")
	}

	// Output:
	// Created: Semaphore(0/2)
	// Initial state
	//   Semaphore: cap=2, acquired=0, available=2
	// After acquiring first token
	//   Semaphore: cap=2, acquired=1, available=1
	// After acquiring second token
	//   Semaphore: cap=2, acquired=2, available=0
	// TryAcquire failed - semaphore full
	//   Semaphore: cap=2, acquired=2, available=0
	// After releasing one token
	//   Semaphore: cap=2, acquired=1, available=1
	// After re-acquiring token
	//   Semaphore: cap=2, acquired=2, available=0
	// Final state - all tokens released
	//   Semaphore: cap=2, acquired=0, available=2
}

// printSemaphore shows you how to inspect a semaphore's state.
// Since Semaphore is implemented as a buffered channel, you can use Go's
// built-in cap() and len() functions rather than needing special methods.
// This gives you a zero-cost abstraction - the semaphore IS the channel.
func printSemaphore(s semaphore.Semaphore, msg string) {
	fmt.Printf("%v\n  Semaphore: cap=%v, acquired=%v, available=%v\n", msg, cap(s), len(s), cap(s)-len(s))
}

// You shouldn't use Semaphore as a raw channel.
// While the underlying type is a channel, the Acquire/Release API handles the nil case specially:
// nil Semaphore means "unlimited" rather than "blocks forever" like nil channels.
// If you bypass this API, you lose this semantic and risk deadlocks.
func Example_antiPattern() {
	sem := semaphore.New(1)
	printSemaphore(sem, "Initial state")

	// You might be tempted to use the semaphore in a select statement for timeouts.
	// DON'T DO THIS. You're bypassing the API and taking on the burden of maintaining
	// the invariant that every send must have exactly one receive.
	select {
	case sem <- struct{}{}:
		printSemaphore(sem, "Acquired via direct channel send (DANGEROUS)")
		// If you forget this receive, you've permanently stolen a token from the pool.
		// Your future Acquire() calls will find fewer tokens than expected.
		defer func() {
			<-sem
			printSemaphore(sem, "Released via direct channel receive")
		}()
	case <-context.Background().Done():
		// This case would handle timeout/cancellation in real code.
		// Instead, consider NOT using this package at-all. It is trivial to implement
		// your own tailored sempahore.
		fmt.Println("Operation cancelled or timed out")
	}

	// Output:
	// Initial state
	//   Semaphore: cap=1, acquired=0, available=1
	// Acquired via direct channel send (DANGEROUS)
	//   Semaphore: cap=1, acquired=1, available=0
	// Released via direct channel receive
	//   Semaphore: cap=1, acquired=0, available=1
}
