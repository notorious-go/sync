package causalorder_test

import (
	"testing"

	"github.com/notorious-go/sync/causalorder"
	"github.com/notorious-go/sync/causalorder/ordertest"
)

// TestPartialOrdering tests that operations for the same key are executed in the
// correct order, while operations for different keys can run concurrently
// without affecting each other.
//
// This test processes the following events in the following order:
//
//  1. A1, which happens immediately.
//  2. B1, which happens immediately.
//  3. A2, which happens after A1.
//  4. B2, which happens after B1.
//  5. A3, which happens after A2 (and transitively after A1).
func TestPartialOrdering(t *testing.T) {
	var ordering causalorder.PartialOrder[string]
	events := []ordertest.Event{
		{
			Token:        "A1",
			Dependencies: nil,
			Operation:    ordering.HappensAfter("A"),
		},
		{
			Token:        "B1",
			Dependencies: nil,
			Operation:    ordering.HappensAfter("B"),
		},
		{
			Token:        "A2",
			Dependencies: []string{"A1"},
			Operation:    ordering.HappensAfter("A"),
		},
		{
			Token:        "B2",
			Dependencies: []string{"B1"},
			Operation:    ordering.HappensAfter("B"),
		},
		{
			Token:        "A3",
			Dependencies: []string{"A2", "A1"},
			Operation:    ordering.HappensAfter("A"),
		},
	}
	ordertest.Test(t, events)
}
