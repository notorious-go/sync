package causalorder

import (
	"sync"
)

// Operation represents a unit of work in a causally ordered execution chain. It
// provides synchronization points for managing when an operation can begin and
// signalling when it has completed.
//
// An Operation is the fundamental building block for enforcing "happens-after"
// relationships in concurrent programs. Each Operation waits for its causal
// dependencies to complete before becoming ready and signals its own completion
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
	// The wait channel is closed when the previous operation in the chain has
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
