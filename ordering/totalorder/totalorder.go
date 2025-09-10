package totalorder

import (
	"sync"

	"github.com/notorious-go/sync/ordering"
)

// TotalOrder enforces a strict sequential ordering of all associated operations.
// This is useful when operations must be serialized globally, regardless of
// their keys or other attributes.
//
// Operations added via HappensNext will execute one at a time in the order they
// were added.
//
// Though trivial, this ordering is useful for composing more complex
// synchronization constructs that apply several total orders.
//
// Most linear chains of operations are better serialized using mutexes,
// channels, or other synchronization primitives. Nonetheless, TotalOrder
// presents a unique synchronization style applicable when operations have a
// definite order of execution and must be serialized.
//
// For example, consider concurrent database writes. If only one write operation
// is allowed to execute at a time, use a semaphore or a mutex to limit
// concurrency when writing to the database. However, if the order of writing ops
// matters, use TotalOrder to ensure that writes are executed in the order they
// were added, while spawning goroutines for each writer.
//
// The TotalOrder is not safe for concurrent use. It must be used from a single
// goroutine that is responsible for defining the definitive order of operations.
// Per the explanation above, it would be meaningless to use it concurrently
// because the order would not be well-defined, thus violating the strictness of
// the total order.
//
// The zero-value TotalOrder is ready to use.
type TotalOrder struct {
	// head is the head of the chain, representing the most recently added operation.
	// It starts as nil and is initialized to a closed channel on first use.
	head chan struct{}
}

// init ensures that the TotalOrder is ready to use. It is safe to call init
// multiple times.
//
// Upon the first call, it sets the head of the chain to a closed channel,
// marking the next operation (by calling HappensNext) as ready to execute
// immediately. Further calls do nothing, as the chain is already initialized.
func (o *TotalOrder) init() {
	// Every new chain starts with a single node, which is closed immediately to
	// signal readiness.
	if o.head == nil {
		o.head = make(chan struct{})
		close(o.head)
	}
}

// HappensNext returns an Operation representing a unit of work that must execute
// after all previously added operations have completed. Operations are executed
// in the exact order that HappensNext was called.
//
// The first operation added to a TotalOrder proceeds immediately without
// waiting. Each following operation waits for its immediate predecessor to call
// Complete() before it can proceed.
//
// IMPORTANT: The caller MUST call Complete() on the returned Operation when the
// operation completes, even if the operation fails or is cancelled. Failing to
// call Complete() will cause all subsequent operations to block indefinitely, which
// may result in memory leaks.
//
// Note that this package only provides ordering guarantees and does not
// propagate cancellation signals between operations. If an operation needs to be
// cancelled, the caller should implement their own cancellation mechanism (such
// as using select with context.Done()) and ensure Complete() is still called to
// unblock the following operations in the chain.
func (o *TotalOrder) HappensNext() ordering.Operation {
	o.init()
	wait := o.head
	done := make(chan struct{})
	o.head = done
	return &totalOperation{
		wait: wait,
		done: done,
	}
}

// A totalOperation implements the ordering.Operation interface for a single totally
// ordered chain of operations.
type totalOperation struct {
	// The wait channel is closed when the previous operation in the chain has
	// completed, allowing this operation to proceed.
	wait <-chan struct{}
	// The done channel is used to signal that the operation associated with this
	// node has completed, thereby allowing the next operation in the chain to
	// proceed.
	done chan struct{}
	// The done channel must be closed only once. Without this flag, calling Complete()
	// twice would've panicked because channels cannot be closed more than once.
	doneOnce sync.Once
}

func (h *totalOperation) Ready() <-chan struct{} {
	return h.wait
}

func (h *totalOperation) Completed() <-chan struct{} {
	return h.done
}

func (h *totalOperation) Complete() {
	h.doneOnce.Do(func() {
		close(h.done)
	})
}
