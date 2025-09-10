package ordertest

import (
	"flag"
	"testing"
)

func TestDisjointEvents(t *testing.T) {
	// All disjoint events don't participate in any happens-after relationship. They
	// should all be able to proceed immediately and in any order. As such, each
	// Event's HappensAfter field is zero.
	events := []Event{
		{Token: "1", Operation: DetachedOp()},
		{Token: "2", Operation: DetachedOp()},
		{Token: "3", Operation: DetachedOp()},
		{Token: "4", Operation: DetachedOp()},
	}
	// Calling ordertest.Test is the actual test since we provide a set of events
	// that we know must pass. Calling ordertest.Test will fail the test if its
	// implementation becomes incorrect and violates this expected behaviour.
	Test(t, events)
}

func TestDependentEvents(t *testing.T) {
	var chain TestChain
	// Dependent events must respect their happens-after relationships. This test
	// defines a simple linear chain of dependencies: 1 -> 2 -> 3 -> 4. As such,
	// each Event's HappensAfter field contains the Token of the previous Event.
	events := []Event{
		{Token: "1", HappensAfter: nil, Operation: chain.Get(0)},
		{Token: "2", HappensAfter: []string{"1"}, Operation: chain.Get(1)},
		{Token: "3", HappensAfter: []string{"2"}, Operation: chain.Get(2)},
		{Token: "4", HappensAfter: []string{"3"}, Operation: chain.Get(3)},
	}
	// Calling ordertest.Test is the actual test since we provide a set of events
	// that we know must pass. Calling ordertest.Test will fail the test if its
	// implementation becomes incorrect and violates this expected behaviour.
	Test(t, events)
}

var xfail = flag.Bool("xfail", false, "run tests that are expected to fail")

func TestBadOrdering(t *testing.T) {
	// This test is expected to fail, so skip it unless explicitly requested with the
	// -xfail flag, in which case we know the user intends to run it to see it fail.
	if !*xfail {
		t.Skip("Skipping test that is expected to fail; use -xfail to run it")
	}

	// This test describes the same linear chain of dependencies as
	// TestDependentEvents, but the Events are set with disjoint operations that do
	// not respect the declared happens-after relationships.
	events := []Event{
		{Token: "1", HappensAfter: nil, Operation: DetachedOp()},
		{Token: "2", HappensAfter: []string{"1"}, Operation: DetachedOp()},
		{Token: "3", HappensAfter: []string{"2"}, Operation: DetachedOp()},
		{Token: "4", HappensAfter: []string{"3"}, Operation: DetachedOp()},
	}
	// This test is expected to fail when passed to ordertest.Test, demonstrating
	// that it correctly identifies violations of the declared ordering constraints.
	Test(t, events)
}

// TestOp is a simple implementation of the Operation interface for testing in
// this package.
//
// It allows tests to describe different readiness and completion scenarios by
// manually controlling the ready and completed channels.
//
// The DetachedOp and ChainedOp helpers initialize the TestOp in common states.
type TestOp struct {
	ready     <-chan struct{}
	completed chan struct{}
}

// DetachedOp returns a TestOp that is ready immediately.
func DetachedOp() *TestOp {
	ch := make(chan struct{})
	close(ch)
	return &TestOp{ready: ch, completed: make(chan struct{})}
}

// Chain returns a TestOp that becomes ready after the given channel is closed.
func ChainedOp(after <-chan struct{}) *TestOp {
	return &TestOp{ready: after, completed: make(chan struct{})}
}

func (o *TestOp) Ready() <-chan struct{} {
	return o.ready
}

func (o *TestOp) Completed() <-chan struct{} {
	return o.completed
}

func (o *TestOp) Complete() {
	close(o.completed)
}

// A TestChain is a helper for creating a sequence of testOps where each
// operation depends on the completion of the previous operation in the chain.
//
// Calling ops.Get(n) returns the nth operation in the chain, creating it if it
// doesn't already exist. Each operation's readiness depends on the completion of
// the previous operation, forming a linear dependency chain.
//
// The first operation in the chain (ops.Get(0)) is ready immediately.
type TestChain map[uint]*TestOp

func (c *TestChain) Get(id uint) *TestOp {
	// Make the zero TestChain usable.
	if *c == nil {
		*c = make(map[uint]*TestOp)
	}
	// Return the existing operation if it already exists.
	if op, ok := (*c)[id]; ok {
		return op
	}
	// Create a new operation, setting its readiness based on the previous
	// operation in the chain.
	op := c.new(id)
	(*c)[id] = op
	return op
}

func (c *TestChain) new(id uint) *TestOp {
	if id == 0 {
		// The first operation is ready immediately.
		return DetachedOp()
	}
	return ChainedOp(c.Get(id - 1).Completed())
}
