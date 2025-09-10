package partialorder_test

import (
	"testing"

	"github.com/notorious-go/sync/ordering/ordertest"
	"github.com/notorious-go/sync/ordering/partialorder"
)

// This test verifies two chains of operations ("alice" and "bob") to ensure that
// operations within the same chain correctly adhere to the interface guarantees.
func TestOperationInterface(t *testing.T) {
	var order partialorder.PartialOrder[string]
	t.Run("alice-in-chains", func(t *testing.T) {
		first := order.HappensAfter("alice")
		second := order.HappensAfter("alice")
		ordertest.TestOperationInterface(t, first, second)
	})
	t.Run("bob-in-chains", func(t *testing.T) {
		first := order.HappensAfter("bob")
		second := order.HappensAfter("bob")
		ordertest.TestOperationInterface(t, first, second)
	})
}

func TestPartialOrdering(t *testing.T) {
	var po partialorder.PartialOrder[string]
	events := []ordertest.Event{
		{
			Token:        "alice:login",
			HappensAfter: nil,
			Operation:    po.HappensAfter("alice"),
		},
		{
			Token:        "bob:login",
			HappensAfter: nil,
			Operation:    po.HappensAfter("bob"),
		},
		{
			Token:        "alice:watch-reels",
			HappensAfter: []string{"alice:login"},
			Operation:    po.HappensAfter("alice"),
		},
		{
			Token:        "bob:binge-reels",
			HappensAfter: []string{"bob:login"},
			Operation:    po.HappensAfter("bob"),
		},
		{
			Token:        "alice:logout",
			HappensAfter: []string{"alice:watch-reels"},
			Operation:    po.HappensAfter("alice"),
		},
		{
			Token:        "bob:logout",
			HappensAfter: []string{"bob:binge-reels"},
			Operation:    po.HappensAfter("bob"),
		},
	}
	ordertest.Test(t, events)
}
