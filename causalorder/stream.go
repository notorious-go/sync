package causalorder

import (
	"context"
	"sync"
	"sync/atomic"
)

type Stream[K comparable] struct {
	chains chainMap[K]
}

// After returns a Barrier that synchronizes after the operations associated with
// the given keys have completed.
func (s *Stream[K]) After(keys ...K) *Barrier {
	h := s.AfterHandle(keys...)
	return newBarrier(h)
}

func (s *Stream[K]) AfterHandle(keys ...K) Handle {
	if len(keys) == 0 {
		// If no keys are provided, we can just return a Handle that is ready immediately
		// and does not require any cleaning up.
		return new(loneHandle)
	}

	if len(keys) == 1 {
		// If only one key is provided, we can just use PartialOrder to create a more
		// efficient Handle for that one key.
		po := (*PartialOrder[K])(&s.chains)
		return po.HappensAfter(keys[0])
	}

	// If multiple keys are provided, we need to create a vector handle that will
	// wait for all of them to become ready and signal completion for each of them
	// when it is marked as done.
	elements := make(map[K]chainNode, len(keys))
	for _, key := range keys {
		elements[key] = s.chains.Advance(key)
	}
	return &vectorHandle[K]{chains: &s.chains, nodes: elements}
}

// A loneHandle is a Handle that is ready immediately, and marking it as done
// affects no other Handle because it does not participate in any specific chain.
//
// Its zero value is ready to use, and it only allocates a channel when Wait() is
// called for the first time.
type loneHandle struct {
	initOnce sync.Once
	waitCh   chan struct{}
}

// Wait does lazy initialization of the wait channel, which is closed
// immediately.
func (h *loneHandle) Wait() <-chan struct{} {
	h.initOnce.Do(func() {
		h.waitCh = make(chan struct{})
		close(h.waitCh)
	})
	return h.waitCh
}

// Done is a no-op because this handle does not participate in any chain.
func (h *loneHandle) Done() {}

// A vectorHandle is a Handle that waits for multiple keys to become ready and
// signals completion for each of them when it is marked as done.
//
// It is used to synchronize an operation that depends on multiple operations
// (associated with the given keys) to complete before it can proceed.
type vectorHandle[K comparable] struct {
	// The map of chains which this handle is associated with. When the handle is
	// Done() and no more operations are pending for this key, the chain will be
	// deleted from this map.
	chains *chainMap[K]
	// Maps each key to its corresponding chain node, which is used to track the
	// state of the operation associated with that key.
	nodes map[K]chainNode

	// The done channels of the vector's nodes must be closed only once. Without this
	// flag, calling Done() twice would've panicked because channels cannot be closed
	// more than once.
	doneOnce sync.Once
	// Waiting for all operations in the vector to complete is done lazily, so that
	// the wait channel is only created when Wait() is called for the first time.
	waitLazily sync.Once
	waitCh     chan struct{}
}

func (h *vectorHandle[K]) Wait() <-chan struct{} {
	h.waitLazily.Do(func() {
		h.waitCh = make(chan struct{})
		if h.shortCircuit() {
			// If all vector elements are already ready, we can close the wait channel
			// immediately, without waiting for all those elements to become ready in a
			// separate goroutine.
			close(h.waitCh)
			return
		}
		go func(elems map[K]chainNode, ready chan struct{}) {
			for _, n := range elems {
				<-n.wait
			}
			close(ready)
		}(h.nodes, h.waitCh)
	})
	return h.waitCh
}

// ShortCircuit checks if all chain elements in the vector are already ready,
// returning true if they are, and false otherwise.
func (h *vectorHandle[K]) shortCircuit() (completed bool) {
	for _, n := range h.nodes {
		select {
		case <-n.wait:
		default:
			// If even a single element is not immediately ready, then the short-circuiting
			// is not possible, and we need to wait for all of them to become ready.
			return false
		}
	}
	return true
}

// Done attempts to free the memory used by chains that are no longer relevant.
// Chains become irrelevant when all operations associated with them have been
// completed.
func (h *vectorHandle[K]) Done() {
	h.doneOnce.Do(func() {
		for k, n := range h.nodes {
			// Clean up the chain for this key if it is no longer necessary.
			//
			// The map's Delete method guarantees that the chain is removed (and its memory
			// reclaimed) only if the node being marked as done is the head of the chain for
			// its key.
			//
			// If the node is not the head, it means that there are still operations waiting
			// for this node to complete, so we must keep the chain alive. Otherwise, any
			// Advance() calls to that chain would be detached from those still in progress
			// operations, which violates the causal order.
			h.chains.Delete(k, n)
			close(n.done)
		}
	})
}

// A Barrier is the synchronization point between causally related operations. It
// is used to ensure that such operations are executed with respect to their
// causal dependencies.
//
// Formally, we say that a Barrier enforces a "happens-after" relation among
// operations in a causally ordered Stream. That is, if the "happens-after"
// relation exists between two operations, the one that happens first must
// complete before the latter one can start.
//
// Users should Await on a Barrier to block until all causal dependencies have
// completed, ensuring the proper serialization of causally related operations.
// Always call the done function returned by Await to prevent memory leaks, even
// if the operation associated with the Barrier failed.
//
// A Barrier is obtained from [Stream.After] and must not be created directly
// because its zero value is meaningless.
//
// The Barrier is safe for concurrent use, though it should be Awaited from a
// single goroutine that is responsible for managing the execution of the
// operation associated with it.
//
// Some unique scenarios need multiple goroutines to wait on the same Barrier.
// For these cases, use the raw Handle ([Barrier.RawHandle]) directly to wait and
// signal completion. Note that the associated operation is only ever completed
// once.
type Barrier struct {
	// The handle is the underlying synchronization primitive that allows waiting for
	// the operation to complete and signalling its completion.
	handle Handle
	// Remembers whether the operation associated with this Barrier has already been
	// completed. This is used to report the completion status because the underlying
	// handle does not provide a way to check if it has been completed.
	completed atomic.Bool
}

func newBarrier(h Handle) *Barrier {
	return &Barrier{handle: h}
}

func (b *Barrier) Await() (done func()) {
	<-b.handle.Wait()
	return b.done
}

func (b *Barrier) AwaitChan(stop <-chan struct{}) (done func(), ready bool) {
	select {
	case <-b.handle.Wait():
		return b.done, true
	case <-stop:
		return b.done, false
	}
}

func (b *Barrier) AwaitContext(ctx context.Context) (done func(), err error) {
	select {
	case <-b.handle.Wait():
		return b.done, nil
	case <-ctx.Done():
		return b.done, ctx.Err()
	}
}

// Done is safe for concurrent use with other methods of Barrier, and it is safe
// to call it multiple times, even if the operation associated with this Barrier
// has already been completed.
func (b *Barrier) done() {
	// The underlying Handle should be safe for concurrent use and may be called
	// multiple times, so we can call Done without worrying about these two
	// constraints.
	b.handle.Done()
	// Marking the operation as completed is also idempotent, so we can safely call
	// it multiple times. It is also safe to call it concurrently with itself and
	// other methods of the Barrier because it uses an atomic boolean.
	b.completed.Store(true)
}

func (b *Barrier) Completed() bool {
	return b.completed.Load()
}

func (b *Barrier) Ready() bool {
	select {
	case <-b.handle.Wait():
		return true
	default:
		return false
	}
}

// RawHandle returns the underlying Handle that can be used to wait for the
// operation associated with this Barrier to be ready and signal its completion.
//
// This is useful for scenarios where multiple goroutines need to wait on the
// same Barrier, or when waiting in a select statement is required.
//
// The barrier reflects the state of the returned handle, and vice versa, so the
// two can be used interchangeably.
func (b *Barrier) RawHandle() Handle {
	return barrierHandle{barrier: b}
}

// A BarrierHandle is a Handle that reflects the state of its associated Barrier.
type barrierHandle struct {
	barrier *Barrier
}

func (h barrierHandle) Wait() <-chan struct{} {
	return h.barrier.handle.Wait()
}

func (h barrierHandle) Done() {
	h.barrier.done()
}
