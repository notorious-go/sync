// Package ordertest provides utilities for testing causal ordering
// implementations. The package offers a framework for verifying that concurrent
// operations respect their declared dependencies when using orderings from the
// ordering packages (e.g. totalorder, partialorder, and causalorder).
//
// # Overview
//
// The primary function [Test] executes a series of [Events] concurrently and
// verifies that all dependency constraints are satisfied.
//
// # Example Usage
//
// Create a test case with dependencies:
//
//	var ordering OrderingType
//	events := []ordertest.Event{
//		{
//			Token:        "user-create",
//			HappensAfter: nil,
//			Operation:    ordering.HappensAfter("user"),
//		},
//		{
//			Token:        "user-login",
//			HappensAfter: []string{"user-create"},
//			Operation:    ordering.HappensAfter("user"),
//		},
//	}
//	ordertest.Test(t, events)
//
// The test will verify that "user-login" only executes after "user-create" has
// completed, even though the goroutines are spawned in reverse order.
package ordertest

import (
	"slices"
	"sync"
	"testing"

	"github.com/notorious-go/sync/ordering"
)

// Test executes an event-loop consisting of the predefined events concurrently
// and verifies that all dependency constraints are satisfied.
//
// The function:
//
//   - Spawns a goroutine for each event in reverse order (to stress the ordering).
//   - Each goroutine waits for its Operation to be ready before proceeding.
//   - Records the actual execution order of events, which is the order in which
//     the spawned goroutines are running.
//   - Verifies that the declared order was satisfied using Event.Check.
//
// This helper is designed to test the correctness of TotalOrder, PartialOrder,
// and CausalOrder implementations by ensuring that operations respect their
// declared dependencies when executing concurrently.
func Test(t *testing.T, events []Event) {
	t.Helper()

	var (
		mu     sync.Mutex
		tokens []string
	)

	var wg sync.WaitGroup
	for _, event := range slices.Backward(events) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer event.Operation.Complete()

			select {
			case <-event.Operation.Ready():
				// The operation is ready, meaning all dependencies are satisfied.
				t.Logf("Processing event %s", event.Token)
			case <-t.Context().Done():
				t.Errorf("test interrupted before event %s could be processed", event.Token)
			}

			// Collect the token for verification.
			mu.Lock()
			tokens = append(tokens, event.Token)
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Verify that all tokens were processed in the expected order.
	for _, event := range events {
		event.Check(t, tokens)
	}
}

// Event represents a step in a concurrent test that uses either
// TotalOrder, PartialOrder, or CausalOrder to test the causal relationships
// between operations.
//
// Each event has a token that identifies it, a list of dependencies that
// represent other events that must complete before this event can be processed,
// and an Operation that defines the "happens-after" relationship for this event.
//
// The ordering types are designed to compose dependencies before the processing
// begins in separate goroutines, which is why definition functions (returning
// the events of test-cases) can prepare the "happens-after" relationships
// without actually executing the operations.
//
// The TestCase runner executes the operations themselves.
type Event struct {
	// Token is a unique identifier for this event, used to track its execution in
	// the processing order and verify dependency constraints.
	Token string

	// HappensAfter lists the tokens of events that must complete before this event
	// should've been processed. The test framework verifies that all dependencies
	// appear before this event in the actual execution order.
	HappensAfter []string

	// Operation represents the ordering operation returned by the ordering types. It
	// provides the synchronization mechanism to enforce the declared dependencies.
	//
	// The test framework waits on the [ordering.Operation.Ready] channel before
	// processing the event, ensuring that declared dependencies are respected.
	//
	// The test framework calls [ordering.Operation.Complete] after processing each
	// event to maintain the correctness of causal chains and prevent deadlocks.
	Operation ordering.Operation
}

// Check verifies that all of this event's dependencies were processed before
// this event in the given execution order.
//
// The tokens parameter should contain the ordered list of event tokens as they
// were actually processed.
//
// This method will verify that:
//   - This event's token appears in the recorded list of tokens.
//   - All dependencies listed in HappensAfter appear before Token in the list.
//
// Any violations of the dependency constraints will be reported as test errors.
func (e Event) Check(t *testing.T, tokens []string) {
	t.Helper()

	// Find the position of this event in the list of tokens.
	eventIndex, ok := e.index(tokens)
	if !ok {
		t.Errorf("event %v was not processed", e.Token)
		return
	}

	// Check that all dependencies appear before this event.
	for _, dep := range e.HappensAfter {
		found := false
		for i := 0; i < eventIndex; i++ {
			if tokens[i] == dep {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("event %v: dependency %v was not processed before it", e.Token, dep)
		}
	}
}

// Finds the index of this event's token in the given slice of tokens.
func (e Event) index(tokens []string) (index int, found bool) {
	for i, token := range tokens {
		if token == e.Token {
			return i, true
		}
	}
	return 0, false
}
