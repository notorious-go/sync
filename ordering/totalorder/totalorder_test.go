package totalorder_test

import (
	"testing"

	"github.com/notorious-go/sync/ordering/ordertest"
	"github.com/notorious-go/sync/ordering/totalorder"
)

func TestOperationInterface(t *testing.T) {
	var order totalorder.TotalOrder
	first := order.HappensNext()
	second := order.HappensNext()
	ordertest.TestOperationInterface(t, first, second)
}

func TestTotalOrderings(t *testing.T) {
	var alice, bob totalorder.TotalOrder
	events := []ordertest.Event{
		{Token: "A1", HappensAfter: nil, Operation: alice.HappensNext()},
		{Token: "B1", HappensAfter: nil, Operation: bob.HappensNext()},
		{Token: "A2", HappensAfter: []string{"A1"}, Operation: alice.HappensNext()},
		{Token: "B2", HappensAfter: []string{"B1"}, Operation: bob.HappensNext()},
		{Token: "A3", HappensAfter: []string{"A2"}, Operation: alice.HappensNext()},
		{Token: "B3", HappensAfter: []string{"B2"}, Operation: bob.HappensNext()},
	}
	ordertest.Test(t, events)
}
