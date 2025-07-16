// Package causalorder provides synchronization primitives for enforcing
// causal ordering of operations in concurrent programs.
//
// The package offers two main ordering strategies:
//
//   - TotalOrder: Ensures all operations execute in a strict sequential order
//   - PartialOrder: Ensures operations with the same key execute sequentially,
//     while operations with different keys can execute concurrently
//
// Both ordering strategies use a handle-based API where Put returns a Handle2
// that must be used to wait for the operation's turn and signal completion.
package causalorder

import (
	"sync"
)

// A Chain is a linked list of channels that allows serializing concurrent
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
func (c *chain) Advance() (wait <-chan struct{}, done chan<- struct{}) {
	// The first time we call Advance, the wait channel is immediately ready.
	if c.head == nil {
		c.head = make(chan struct{})
		close(c.head) // Close the channel immediately to signal readiness.
	}

	// The wait channel is the head of the chain set by the last Put call.
	// The done channel is the new head of the chain, which will be used by
	// callers to signal completion for the next Put call.
	wait = c.head
	c.head = make(chan struct{})
	done = c.head
	return wait, done
}

type chainHandle struct {
	// The wait channel is closed when all previous operations in the chain have
	// completed, allowing the next operation (the one associated with this handle)
	// to proceed.
	wait <-chan struct{}
	// The done channel is used to signal that the operation associated with this
	// handle has completed, thereby allowing the next operation in the chain to
	// proceed.
	done chan<- struct{}
	// The done channel must be closed only once. Without this flag, calling Done()
	// twice would've panicked.
	doneOnce sync.Once
}

func (h *chainHandle) Wait() <-chan struct{} {
	return h.wait
}

func (h *chainHandle) Done() {
	h.doneOnce.Do(func() {
		close(h.done)
	})
}

type Handle interface {
	// Wait returns a channel that is closed when it is this handle's turn to
	// execute.
	Wait() <-chan struct{}
	// Done signals that the operation associated with this handle has completed,
	// thereby allowing causally dependent operations to proceed.
	//
	// The Done method must be called at least once, otherwise the program may leak
	// resources further down the dependency graph.
	//
	// Done may be called multiple times, though calling it more than once is a
	// no-op.
	Done()
}

// TODO: add a function returning true after Done() is called

// TotalOrder enforces a strict sequential ordering of all associated operations.
// This is useful when operations must be serialized globally, regardless of
// their keys or other attributes.
//
// Operations added via Put will execute one at a time in the order they were
// added.
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

// TODO Put adds an operation to the total order queue and returns a Handle2.
// The caller must wait on the handle before executing the operation,
// and call Done when the operation completes.
func (o *TotalOrder) HappensAfter() Handle {
	wait, done := (*chain)(o).Advance()
	return &chainHandle{wait: wait, done: done}
}

// A chainMap is a map of chains by keys, where each key corresponds to a
// separate chain of operations, which is safe for concurrent use.
//
// The zero-value chainMap is ready to use.
type chainMap[K comparable] struct {
	initOnce sync.Once
	// We must protect the map of chains against concurrent access.
	//
	// For example, multiple calls to Handle.Done() may happen concurrently with each
	// other, or with Put calls.
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

func (m *chainMap[K]) Advance(key K) (wait <-chan struct{}, done chan<- struct{}) {
	chains, release := m.acquire()
	defer release()
	if _, exists := chains[key]; !exists {
		chains[key] = &chain{}
	}
	return chains[key].Advance()
}

// Delete removes the chain associated with the given key from the map only if
// the head of the chain is the given head (done) channel. The given head channel
// is expected to be the done channel returned by a prior call to Advance() for
// the same key.
//
// If the chain is already deleted or if the head is not the given head (done)
// channel, the function does nothing.
func (m *chainMap[K]) Delete(key K, head chan<- struct{}) {
	chains, release := m.acquire()
	defer release()
	if c, ok := chains[key]; ok && c.head == head {
		delete(chains, key)
	}
}

// PartialOrder enforces a strict sequential ordering for operations with the
// same key, while allowing operations with different keys to execute
// concurrently. This is useful for scenarios where operations must be serialized
// per key, such as when processing events that are grouped by an identifier.
//
// Operations added via Put will execute in the order they were added for each
// key, but operations with different keys can run in parallel.
//
// The PartialOrder is not safe for concurrent use. It must be used from a single
// goroutine that is responsible for managing the execution of operations
// associated with it. This is because the order of operations is defined by the
// order in which they are added to the PartialOrder.
//
// The zero value is ready to use.
type PartialOrder[K comparable] chainMap[K]

// TODO Put adds an operation for the given key to the partial order queue and returns a Handle2.
// Operations with the same key will execute sequentially in the order they were added.
// Operations with different keys may execute concurrently.
//
// The caller must wait on the handle before executing the operation,
// and call Done when the operation completes.
//
// When the last pending operation for a key completes, the key's queue is
// automatically cleaned up to prevent memory leaks.
func (o *PartialOrder[K]) HappensAfter(key K) Handle {
	namespace := (*chainMap[K])(o)
	wait, done := namespace.Advance(key)
	return &partialHandle[K]{
		key:         key,
		namespace:   namespace,
		chainHandle: chainHandle{wait: wait, done: done},
	}
}

type partialHandle[K comparable] struct {
	// The key with which this handle is associated in the namespace.
	key K
	// The map of chains which this handle is associated with. When the handle is
	// Done() and no more operations are pending for this key, the chain will be
	// deleted from this map.
	namespace *chainMap[K]
	// A partialHandle embeds a chainHandle to reuse the same Wait() but with a
	// different Done() implementation.
	chainHandle
}

func (h *partialHandle[K]) Done() {
	h.doneOnce.Do(func() {
		close(h.done)
		h.namespace.Delete(h.key, h.done)
	})
}
