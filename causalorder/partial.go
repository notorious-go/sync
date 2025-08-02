package causalorder

import (
	"sync"
)

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
