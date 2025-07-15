// Package causalorder provides synchronization primitives for enforcing
// causal ordering of operations in concurrent programs.
//
// The package offers two main ordering strategies:
//
//   - TotalOrder: Ensures all operations execute in a strict sequential order
//   - PartialOrder: Ensures operations with the same key execute sequentially,
//     while operations with different keys can execute concurrently
//
// Both ordering strategies use a handle-based API where Put returns a Handle
// that must be used to wait for the operation's turn and signal completion.
package causalorder

import (
	"sync"
)

// A Chain is a linked list of channels that allows
//
type chain struct {
	head chan struct{}
}

// Put is actually Next/ADVANCE/Increment?
func (c *chain) Put() (wait <-chan struct{}, done chan<- struct{}) {
	// The first time we call Put, the wait channel is immediately ready.
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

// Handle represents a position in an ordering queue.
// It provides methods to wait for an operation's turn and signal completion.
//
// A Handle must call Done when the operation completes to allow
// later operations to proceed.
type Handle struct {
	wait     <-chan struct{}
	done     chan<- struct{}
	whenDone func()
}

// Wait returns a channel that is closed when it is this handle's turn
// to execute. The channel can be used in a select statement:
//
//	<-handle.Wait()
//	// Now it's our turn to execute
//
// Or to wait with a timeout:
//
//	select {
//	case <-handle.Wait():
//		// Proceed with operation
//	case <-time.After(timeout):
//		// Handle timeout
//	}
func (h *Handle) Wait() <-chan struct{} {
	return h.wait
}

// Done signals that the operation associated with this handle has completed.
// This allows the next operation in the queue to proceed.
//
// Done must be called exactly once when the operation completes.
// Failing to call Done will block all subsequent operations in the queue.
func (h *Handle) Done() {
	close(h.done)
	if h.whenDone != nil {
		h.whenDone()
	}
}

// Ready reports whether it is this handle's turn to execute.
// It returns immediately without blocking.
//
// This is equivalent to:
//
//	select {
//	case <-h.Wait():
//		return true
//	default:
//		return false
//	}
func (h *Handle) Ready() bool {
	select {
	case <-h.wait:
		return true
	default:
		return false
	}
}

// TODO: add a function returning true after Done() is called

// TotalOrder enforces a strict sequential ordering of all operations.
// Operations added via Put will execute one at a time in the order they were added.
//
// Example:
//
//	var order TotalOrder
//
//	// In goroutine 1
//	handle1 := order.Put()
//	<-handle1.Wait()
//	// Do work...
//	handle1.Done()
//
//	// In goroutine 2
//	handle2 := order.Put()
//	<-handle2.Wait()  // This blocks until handle1.Done() is called
//	// Do work...
//	handle2.Done()
//
// The zero value is ready to use.
type TotalOrder struct {
	mu    sync.Mutex
	chain chain
}

// Put adds an operation to the total order queue and returns a Handle.
// The caller must wait on the handle before executing the operation,
// and call Done when the operation completes.
//
// Put is safe to call concurrently from multiple goroutines.
func (o *TotalOrder) Put() Handle {
	o.mu.Lock()
	defer o.mu.Unlock()
	wait, done := o.chain.Put()
	return Handle{wait: wait, done: done}
}

// PartialOrder enforces sequential ordering for operations with the same key,
// while allowing operations with different keys to execute concurrently.
//
// This is useful when operations must be serialized per resource (identified by key)
// but can run in parallel across different resources.
//
// Example:
//
//	var order PartialOrder
//
//	// Operations on "resource1" execute sequentially
//	handle1 := order.Put("resource1")
//	handle2 := order.Put("resource1")
//
//	// Operations on "resource2" can execute concurrently with "resource1"
//	handle3 := order.Put("resource2")
//
// The zero value is ready to use.
type PartialOrder struct {
	mu     sync.Mutex
	chains map[string]*chain
}

// Put adds an operation for the given key to the partial order queue and returns a Handle.
// Operations with the same key will execute sequentially in the order they were added.
// Operations with different keys may execute concurrently.
//
// The caller must wait on the handle before executing the operation,
// and call Done when the operation completes.
//
// When the last pending operation for a key completes, the key's queue is
// automatically cleaned up to prevent memory leaks.
//
// Put is safe to call concurrently from multiple goroutines.
func (o *PartialOrder) Put(key string) Handle {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.chains == nil {
		o.chains = make(map[string]*chain)
	}
	if _, exists := o.chains[key]; !exists {
		o.chains[key] = &chain{}
	}
	wait, done := o.chains[key].Put()
	return Handle{
		wait: wait,
		done: done,
		whenDone: func() {
			o.mu.Lock()
			defer o.mu.Unlock()
			chain, ok := o.chains[key]
			if !ok {
				// Chain already deleted. This can happen if Handle.Done() is called
				// multiple times, or out of order. This is not the common case.
				return
			}
			if chain.head != done {
				// If the head is not the done channel that was just closed,
				// it means another Put is in progress, so we should not delete this chain.
				return
			}
			delete(o.chains, key)
		},
	}
}

// WaitAll waits for all provided handles to be ready.
// It returns a channel that is closed when all handles are ready,
// and a cancel function that can be used to abort the wait.
//
// This is useful for coordinating multiple operations that must all
// be ready before proceeding:
//
//	handle1 := order1.Put()
//	handle2 := order2.Put()
//	handle3 := order3.Put()
//
//	ready, cancel := WaitAll(handle1, handle2, handle3)
//	defer cancel()
//
//	select {
//	case <-ready:
//		// All handles are ready
//	case <-ctx.Done():
//		// Context cancelled
//	}
//
// The cancel function should be called when the wait is no longer needed
// to free resources.
func WaitAll(handles ...Handle) (c <-chan struct{}, cancel func()) {
	quit := make(chan struct{}, 1)
	ready := make(chan struct{})
	go func() {
		for _, h := range handles {
			select {
			case <-quit:
				return
			case <-h.Wait():
			}
		}
		close(ready)
	}()
	return ready, func() {
		close(quit)
	}
}

type VectorOrder[K comparable] struct {
	mu     sync.Mutex
	chains map[K]*chain
}

func (o *VectorOrder[K]) Put(keys ...K) Handle {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.chains == nil {
		o.chains = make(map[K]*chain)
	}

	wait := make([]<-chan struct{}, len(keys))
	done := make([]chan<- struct{}, len(keys))
	for i, key := range keys {
		if _, exists := o.chains[key]; !exists {
			o.chains[key] = &chain{}
		}
		wait[i], done[i] = o.chains[key].Put()
	}
	return vectorHandle{
		waits: wait,
		dones: done,
	}
}

type vectorHandle struct {
	once  sync.Once
	ch    chan struct{}
	waits []<-chan struct{}
	dones []chan<- struct{}
}

func (h vectorHandle) Wait() <-chan struct{} {
	h.once.Do(func() {
		h.ch = make(chan struct{})
		go func() {
			for _, w := range h.waits {
				<-w
			}
			close(h.ch)
		}()
	})
	return h.ch
}

func (h vectorHandle) Done() {
	for _, d := range h.dones {
		close(d)
	}
}

type handler interface {
	Wait() <-chan struct{}
	Done()
}
