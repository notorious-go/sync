package causalorder_test

import (
	"testing"

	"github.com/notorious-go/sync/causalorder"
	"github.com/notorious-go/sync/causalorder/ordertest"
)

// TestTotalOrdering tests that all operations are executed in a total order,
//
// This test processes the following events in the following order:
//
//  1. 1️⃣, which happens immediately.
//  2. 2️⃣, which happens after 1️⃣.
//  3. 3️⃣, which happens after 2️⃣, and transitively after 1️⃣.
//  4. 4️⃣, which happens after 3️⃣, and transitively after 2️⃣ and 1️⃣.
func TestTotalOrdering(t *testing.T) {
	var ordering causalorder.TotalOrder
	var events = []ordertest.Event{
		{
			Token:        "1️⃣",
			Dependencies: nil,
			Operation:    ordering.HappensAfter(),
		},
		{
			Token:        "2️⃣",
			Dependencies: []string{"1️⃣"},
			Operation:    ordering.HappensAfter(),
		},
		{
			Token:        "3️⃣",
			Dependencies: []string{"2️⃣", "1️⃣"},
			Operation:    ordering.HappensAfter(),
		},
		{
			Token:        "4️⃣",
			Dependencies: []string{"3️⃣", "2️⃣", "1️⃣"},
			Operation:    ordering.HappensAfter(),
		},
	}
	ordertest.Test(t, events)
}
