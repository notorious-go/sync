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
	"context"
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

// A Barrier is the synchronization point between causally related operations. It
// is used to ensure that such operations are executed with respect to their
// causal dependencies.
//
// Formally, we say that a Barrier enforces a "happens-after" relation among
// operations in a causal order. That is, if the "happens-after" relation exists
// between two operations, the one that happens first must complete before the
// latter one can start.
//
// Iin causal ordering chains. It blocks
// operations until all their dependencies have completed, ensuring the proper
// linearization among concurrent operations.
//
// A Barrier is obtained from TotalOrder, PartialOrder, or VectorOrder
// and must not be created directly.
//
// The Barrier is not safe for concurrent use. It must be used from a single
// goroutine that is responsible for managing the execution of the operations
// associated with it.
//
// Some unique scenarios need multiple goroutines to wait on the same Barrier.
// For these cases, use the raw Handle within the Barrier to wait and signal
// completion. Note that the operation is only ever completed once.
type Barrier struct {
	// The handle is the underlying synchronization primitive that allows waiting for
	// the operation to complete and signalling its completion.
	handle Handle
	// Remembers whether the operation associated with this Barrier has already been
	// completed. This is used to report the completion status because the underlying
	// handle does not provide a way to check if it has been completed.
	completed bool
}

func newBarrier(h Handle) *Barrier {
	return &Barrier{handle: h}
}

func (b *Barrier) Await(stop <-chan struct{}) (done func(), ready bool) {
	select {
	case <-b.handle.Wait():
		return b.done, true
	case <-stop:
		return noop, false
	}
}

func (b *Barrier) AwaitContext(ctx context.Context) (done func(), err error) {
	select {
	case <-b.handle.Wait():
		return b.done, nil
	case <-ctx.Done():
		return noop, ctx.Err()
	}
}

func noop() {}

func (b *Barrier) done() {
	// No need to ensure that Done is called only once, as Handle implementations
	// must support multiple Done calls.
	b.handle.Done()
	// Marking the operation as completed is also idempotent, so we can safely call
	// it multiple times.
	b.completed = true
}

func (b *Barrier) Ready() bool {
	select {
	case <-b.handle.Wait():
		return true
	default:
		return false
	}
}

func (b *Barrier) Completed() bool {
	return b.completed
}

// RawHandle returns the underlying Handle that can be used to wait for the
// operation associated with this Barrier to be ready and signal its completion.
//
// This is useful for scenarios where multiple goroutines need to wait on the
// same Barrier, or when waiting in a select statement is required.
//
// The barrier reflects the state of the returned handle, so the two can be used
// interchangeably.
func (b *Barrier) RawHandle() Handle {
	return rawHandle{
		wait: b.handle.Wait(),
		done: b.done,
	}
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

type rawHandle struct {
	wait <-chan struct{}
	// The done callback must be idempotent, i.e. can be called multiple times
	// without side effects.
	done func()
}

func (h rawHandle) Wait() <-chan struct{} {
	return h.wait
}

func (h rawHandle) Done() {
	h.done()
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
func (o *TotalOrder) AfterHandle() Handle {
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
func (o *PartialOrder[K]) AfterHandle(key K) Handle {
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

type VectorOrder[K comparable] struct {
	chains chainMap[K]
}

// After returns a Barrier that synchronizes after the operations associated with
// the given keys have completed.
func (o *VectorOrder[K]) After(keys ...K) *Barrier {
	h := o.AfterHandle(keys...)
	return newBarrier(h)
}

func (o *VectorOrder[K]) AfterHandle(keys ...K) Handle {
	if len(keys) == 0 {
		// If no keys are provided, we can just return a Handle that is ready immediately
		// and does not require any cleaning up.
		return new(TotalOrder).Put()
		//return &chainlessHandle{}
	}

	if len(keys) == 1 {
		// If only one key is provided, we can just use the PartialOrder's Put method to
		// a more efficient Handle for that key.
		po := (*PartialOrder[K])(&o.chains)
		return po.AfterHandle(keys[0])
	}

	// If multiple keys are provided, we need to create a vector handle that will
	// wait for all of them to become ready and signal completion for each of them
	// when it is done.
	elements := make([]vectorElem[K], len(keys))
	for i, key := range keys {
		wait, done := o.chains.Advance(key)
		elements[i] = vectorElem[K]{
			key:  key,
			wait: wait,
			done: done,
		}
	}
	return &vectorHandle[K]{
		namespace: &o.chains,
		elements:  elements,
	}
}

type vectorHandle[K comparable] struct {
	namespace *chainMap[K]
	elements  []vectorElem[K]

	waitOnce sync.Once
	waitCh   chan struct{}
	doneOnce sync.Once
}

type vectorElem[K comparable] struct {
	key  K
	wait <-chan struct{}
	done chan<- struct{}
}

func (h *vectorHandle[K]) Wait() <-chan struct{} {
	h.waitOnce.Do(func() {
		h.waitCh = make(chan struct{})
		if h.shortCircuit() {
			// If all vector elements are already ready, we can close the wait channel
			// immediately, without waiting for all those elements to become ready in a
			// separate goroutine.
			close(h.waitCh)
			return
		}
		go func(elems []vectorElem[K], ready chan struct{}) {
			for i := range elems {
				<-elems[i].wait
			}
			close(ready)
		}(h.elements, h.waitCh)
	})
	return h.waitCh
}

func (h *vectorHandle[K]) shortCircuit() (completed bool) {
	for i := range h.elements {
		select {
		case <-h.elements[i].wait:
		default:
			// If even a single element is not immediately ready, then the short-circuiting
			// is not possible, and we need to wait for all of them to become ready.
			return false
		}
	}
	return true
}

func (h *vectorHandle[K]) Done() {
	h.doneOnce.Do(func() {
		chains, release := h.namespace.acquire()
		defer release()
		for _, elem := range h.elements {
			close(elem.done)
			// Clean up the chain for this key if it is no longer necessary.
			//
			// If the done channel is not the head of the chain, it means that there are
			// still operations pending for this key, so we should not delete the chain yet.
			if c, ok := chains[elem.key]; ok && c.head == elem.done {
				// When the done channel is the head of the chain, we can delete the chain for
				// this key, as it means that no more ongoing operations are pending completion
				// for this key.
				delete(chains, elem.key)
			}
		}
	})
}

type chainlessHandle struct {
	initOnce sync.Once
	waitCh   chan struct{}
}

func (h *chainlessHandle) Wait() <-chan struct{} {
	h.initOnce.Do(func() {
		h.waitCh = make(chan struct{})
		close(h.waitCh)
	})
	return h.waitCh
}

func (h *chainlessHandle) Done() {}
