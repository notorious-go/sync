// Package causalorder provides synchronization primitives for enforcing
// causal ordering of operations in concurrent programs.
//
// The package offers three main ordering strategies:
//
//   - [TotalOrder]: Ensures all operations execute in a strict sequential order.
//   - [PartialOrder]: Ensures operations with the same key execute sequentially,
//     while operations with different keys can execute concurrently.
//   - [VectorOrder]: Ensure operations execute in a causal order based on their
//     keys, allowing for concurrent execution of operations with multiple different
//     keys.
//
// All ordering strategies use an Operation-based API where HappensAfter returns
// an [Operation] that must be completed before the next operations can proceed.
//
// When working with operations, it is important to call [Operation.Complete]
// when the operation finishes, regardless of whether it succeeds, fails, or is
// cancelled. This is crucial to maintain the correctness of causal chains which
// prevents deadlocks, memory, and goroutine leaks.
//
// Note that this package only provides ordering guarantees and does not
// propagate cancellation or error states between operations. If an operation
// fails, the caller should implement their own error handling while still
// calling [Operation.Complete] to maintain the correctness of causal chains. For
// example, if an operation needs to be cancelled, the caller should implement
// their own cancellation mechanism (such as using select with context.Done())
// and ensure [Operation.Complete] is still called to unblock the following
// cancelled operations in the chain.
package causalorder

import (
	"sync"
)

// A chain is a linked list of channels that allows serializing concurrent
// operations into a linear order, wherein each operation must wait for all
// previous operations to complete before beginning execution.
//
// The zero-value Chain is ready to use.
type chain struct {
	head chan struct{}
}

// Advance creates a new head for the chain and returns two channels:
//
//   - wait: is ready when the operation associated with the previous head has
//     completed.
//   - done: should be closed when the operation associated with the new head is
//     complete.
//
// The first time Advance is called, the returned wait channel is immediately
// ready, allowing the first operation associated with this chain to proceed
// without waiting.
func (c *chain) Advance() chainNode {
	// The first time we call Advance, the wait channel is immediately ready.
	if c.head == nil {
		c.head = make(chan struct{})
		close(c.head) // Close the channel immediately to signal readiness.
	}

	// The wait channel is the head of the chain set by the last Advance call. The
	// done channel is the new head of the chain, which callers will use to signal
	// completion for the next Advance call.
	wait := c.head
	c.head = make(chan struct{})
	done := c.head
	return chainNode{wait: wait, done: done}
}

// IsHead checks if the given node is the head of the chain.
//
// If the given node is the head of the chain, it means that there are no pending
// operations in this chain that are waiting for its completion. In other words,
// it indicates that if the operation associated with that node is done, this
// chain is equivalent to an empty (zero-value) chain.
//
// This function is valid to call even if the chain is nil, in which case it
// returns false, indicating that the node cannot be the head of a nil chain.
func (c *chain) IsHead(node chainNode) bool {
	if c == nil {
		return false
	}
	return c.head == node.done
}

// A chainNode holds the channels that represent the state of an operation in a
// causally ordered chain. Nodes in the chain are linked together such that each
// node's wait channel is the done channel of the previous node.
type chainNode struct {
	// The wait channel is closed when the previous operation in the chain have
	// completed, allowing this operation to proceed.
	//
	// This channel is the done channel of the previous node in the chain, or
	// a newly created channel if this is the first node in the chain.
	wait <-chan struct{}
	// The done channel is used to signal that the operation associated with this
	// node has completed, thereby allowing the next operation in the chain to
	// proceed.
	done chan struct{}
}

// A chainMap is a map of chains by keys, where each key corresponds to a
// separate chain of operations, which is safe for concurrent use.
//
// The zero-value chainMap is ready to use.
type chainMap[K comparable] struct {
	// Makes the zero-value chainMap ready to use and concurrent-safe.
	initOnce sync.Once
	// We must protect the map of chains against concurrent access.
	//
	// For example, multiple calls to Operation.Complete() may happen concurrently with each
	// other, or with HappensAfter calls.
	chains chan map[K]*chain
}

// Note that the acquired map is safe for modification until the release function
// is called.
//
// We return a map and not a pointer to it because map values are just pointers
// to the underlying hmap structure on the heap.
func (m *chainMap[K]) acquire() (chains map[K]*chain, release func()) {
	m.initOnce.Do(func() {
		m.chains = make(chan map[K]*chain, 1)
		m.chains <- make(map[K]*chain)
	})
	chains = <-m.chains
	release = func() { m.chains <- chains }
	return chains, release
}

// Advance creates a new head for the chain associated with the given key and
// returns it.
//
// If the key does not exist in the map, a new chain is created for it. A key
// without a chain is equivalent to a chain with its head marked as complete,
// meaning that the next operation associated with that key can proceed
// immediately.
func (m *chainMap[K]) Advance(key K) (head chainNode) {
	chains, release := m.acquire()
	defer release()
	if _, exists := chains[key]; !exists {
		chains[key] = new(chain)
	}
	return chains[key].Advance()
}

// Delete removes the chain associated with the given key from the map only if
// the head of the chain is the given head node. The given head node is expected
// to be returned by a prior call to Advance() for the same key.
//
// After a chain is deleted, further calls to Advance() for the same key will
// create a new chain, effectively resetting the state for that key. Any inflight
// operations waiting for nodes in the deleted chain will not be affected, as
// they will continue to wait for their respective dependencies to complete.
//
// If the chain is already deleted or if the head is not the given node, the
// function does nothing.
func (m *chainMap[K]) Delete(key K, head chainNode) {
	chains, release := m.acquire()
	defer release()
	// If the given node is indeed the head of the chain (associated with the given
	// key), it means that there are no inflight operations waiting for that head. In
	// which case, we can safely delete the chain from this map.
	if c, ok := chains[key]; ok && c.IsHead(head) {
		// Deleting the chain from this map is equivalent to resetting the chain to its
		// zero value, which is ready to use.
		delete(chains, key)
	}
}

// Operation represents a unit of work in a causally ordered execution chain. It
// provides synchronization points for managing when an operation can begin and
// signaling when it has completed.
//
// An Operation is the fundamental building block for enforcing "happens-after"
// relationships in concurrent programs. Each Operation waits for its causal
// dependencies to complete before becoming ready, and signals its own completion
// to unblock dependent operations.
//
// The Operation interface provides a channel-based API that enables flexible
// synchronization patterns:
//   - Block until ready using <-op.Ready()
//   - Select with multiple channels: select { case <-op.Ready(): ... case <-ctx.Done(): ... }
//   - Check readiness without blocking: select { case <-op.Ready(): ... default: ... }
//   - Monitor completion: <-op.Completed()
//
// Operations are safe for concurrent use. While typically a single goroutine
// manages an operation's lifecycle, the channels can be safely accessed from
// multiple goroutines when coordination is needed.
type Operation interface {
	// Ready returns a channel that closes when all causal dependencies have
	// completed, signalling that this operation may begin execution.
	//
	// The channel is closed exactly once and remains closed thereafter. Multiple
	// goroutines may safely wait on this channel.
	//
	// For the first operation in a chain, this channel is closed immediately,
	// allowing it to proceed without waiting.
	Ready() <-chan struct{}

	// Completed returns a channel that closes when this operation has been marked
	// as complete via the Complete method.
	//
	// The channel is closed when Complete is called for the first time and
	// remains closed thereafter.
	Completed() <-chan struct{}

	// Complete marks this operation as finished, closing the Completed channel and
	// allowing any causally dependent operations to proceed.
	//
	// This method MUST be called at least once when the operation finishes, whether
	// it succeeds, fails, or is cancelled. Failing to call Complete will cause all
	// dependent operations to block indefinitely, potentially causing goroutine
	// and memory leaks.
	//
	// Users are encouraged to use defer op.Complete() immediately after starting an
	// operation to ensure completion even in case of errors or early returns.
	//
	// Complete is safe to call multiple times - subsequent calls are no-ops.
	// However, for clarity and correctness, it should typically be called exactly
	// once per operation.
	Complete()
}

// TotalOrder enforces a strict sequential ordering of all associated operations.
// This is useful when operations must be serialized globally, regardless of
// their keys or other attributes.
//
// Operations added via HappensAfter will execute one at a time in the order they
// were added.
//
// Though trivial, this ordering is useful for composing more complex
// synchronization constructs that compose several total orders.
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
type TotalOrder chain

// HappensAfter returns an Operation representing a unit of work that must execute
// after all previously added operations have completed. Operations are executed
// in the exact order that HappensAfter was called.
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
func (o *TotalOrder) HappensAfter() Operation {
	n := (*chain)(o).Advance()
	return &totalOperation{chainNode: n}
}

// A totalOperation implements the Operation interface for a single totally
// ordered chain of operations.
type totalOperation struct {
	chainNode
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

// PartialOrder enforces a strict sequential ordering for operations with the
// same key, while allowing operations with different keys to execute
// concurrently. This is useful for scenarios where operations must be serialized
// per key, such as when processing events that are grouped by an identifier.
//
// Operations added via HappensAfter will execute in the order they were added
// for each key, but operations with different keys can run in parallel.
//
// The PartialOrder is not safe for concurrent use. It must be used from a single
// goroutine that is responsible for managing the execution of operations
// associated with it. This is because the order of operations is defined by the
// order in which they are added to the PartialOrder.
//
// The zero-value PartialOrder is ready to use.
type PartialOrder[K comparable] chainMap[K]

// HappensAfter returns an Operation representing a unit of work that must execute
// after all previously added operations for the given key have completed.
// Operations with the same key are executed in the exact order that HappensAfter
// was called, while operations with different keys may execute concurrently.
//
// The first operation for a given key proceeds immediately without waiting. Each
// following operation for that key waits for its immediate predecessor with the
// same key to call Complete() before it can proceed.
//
// IMPORTANT: The caller MUST call Complete() on the returned Operation when the
// operation completes, even if the operation fails or is cancelled. Failing to
// call Complete() will cause all subsequent operations for that key to block
// indefinitely, which may result in memory leaks.
//
// When the last pending operation for a key completes, the key's internal chain
// is automatically cleaned up to prevent memory leaks.
//
// Note that this package only provides ordering guarantees and does not
// propagate cancellation signals between operations. If an operation needs to be
// cancelled, the caller should implement their own cancellation mechanism (such
// as using select with context.Done()) and ensure Complete() is still called to
// unblock the following operations for that key.
func (o *PartialOrder[K]) HappensAfter(key K) Operation {
	chains := (*chainMap[K])(o)
	head := chains.Advance(key)
	return &partialOperation[K]{
		chainNode: head,
		key:       key,
		chains:    chains,
	}
}

type partialOperation[K comparable] struct {
	chainNode
	// The done channel must be closed only once. Without this flag, calling Complete()
	// twice would've panicked because channels cannot be closed more than once.
	doneOnce sync.Once

	// The key with which this operation is associated in the namespace of chains.
	key K
	// The map of chains which this operation is associated with. When the operation is
	// Complete() and no more operations are pending for this key, the chain will be
	// deleted from this map.
	chains *chainMap[K]
}

func (h *partialOperation[K]) Ready() <-chan struct{} {
	return h.wait
}

func (h *partialOperation[K]) Completed() <-chan struct{} {
	return h.done
}

func (h *partialOperation[K]) Complete() {
	h.doneOnce.Do(func() {
		close(h.done)
		h.chains.Delete(h.key, h.chainNode)
	})
}

// Await is a convenience function that blocks until the given Operation is ready
// to execute and returns a done function that must be called to mark the
// operation as complete.
//
// This function provides syntactic sugar for the common pattern of waiting for
// an operation to be ready and then marking it as complete:
//
//	done := causalorder.Await(op)
//	defer done()
//	// ... perform operation ...
//
// This is equivalent to:
//
//	<-op.Ready()
//	defer op.Complete()
//	// ... perform operation ...
//
// The returned done function is safe to call multiple times and should always be
// called to prevent blocking further operations in the causal chain.
func Await(op Operation) (done func()) {
	<-op.Ready()
	return op.Complete
}
