package causalorder

import (
	"sync"
)

type VectorOrder[K comparable] struct {
	chains chainMap[K]
}

// HappensAfter returns an Operation that synchronizes after the operations
// associated with the given keys have completed.
func (s *VectorOrder[K]) HappensAfter(keys ...K) Operation {
	if len(keys) == 0 {
		// If no keys are provided, we can just return an Operation that is ready
		// immediately and does not require any cleaning up.
		return new(loneOperation)
	}

	if len(keys) == 1 {
		// If only one key is provided, we can just use PartialOrder to create a more
		// efficient Operation for that one key.
		po := (*PartialOrder[K])(&s.chains)
		return po.HappensAfter(keys[0])
	}

	// If multiple keys are provided, we need to create a vector operation that will
	// wait for all of them to become ready and signal completion for each of them
	// when it is marked as complete.
	dependencies := make(map[K]chainNode, len(keys))
	for _, key := range keys {
		dependencies[key] = s.chains.Advance(key)
	}
	return &vectorOperation[K]{chains: &s.chains, nodes: dependencies}
}

// A loneOperation is an Operation that is ready immediately, and marking it as
// complete affects no other Operation because it does not participate in any
// specific chain.
//
// Its zero value is ready to use, and it only allocates channels when Ready() or
// Completed() are called for the first time.
type loneOperation struct {
	init      sync.Once
	ready     chan struct{}
	completed chan struct{}
	complete  sync.Once
}

func (o *loneOperation) initOnce() {
	o.init.Do(func() {
		o.ready = make(chan struct{})
		close(o.ready) // Close the ready channel immediately, as this operation is ready to use.
		o.completed = make(chan struct{})
	})
}

func (o *loneOperation) Ready() <-chan struct{} {
	o.initOnce()
	return o.ready
}

func (o *loneOperation) Completed() <-chan struct{} {
	o.initOnce()
	return o.completed
}

func (o *loneOperation) Complete() {
	o.complete.Do(func() {
		close(o.completed)
	})
}

// A vectorOperation is an Operation that waits for multiple keys to become ready
// and signals completion for each of them when it is marked as complete.
//
// It is used to synchronize an operation that depends on multiple operations
// (associated with the given keys) to complete before it can proceed.
type vectorOperation[K comparable] struct {
	// The map of chains which this operation is associated with. When the operation
	// is Complete() and no more operations are pending for this key, the chain will
	// be deleted from this map.
	chains *chainMap[K]
	// Maps each key to its corresponding chain node, which is used to track the
	// state of the operation associated with that key.
	nodes map[K]chainNode

	// The done channels of the vector's nodes must be closed only once. Without this
	// flag, calling Complete() twice would've panicked because channels cannot be closed
	// more than once.
	doneOnce sync.Once
	// Waiting for all operations in the vector to complete is done lazily, so that
	// the ready channel is only created when Ready() is called for the first time.
	readyLazily sync.Once
	readyCh     chan struct{}
}

func (h *vectorOperation[K]) Ready() <-chan struct{} {
	h.readyLazily.Do(func() {
		h.readyCh = make(chan struct{})
		if h.shortCircuit() {
			// If all vector elements are already ready, we can close the ready channel
			// immediately, without waiting for all those elements to become ready in a
			// separate goroutine.
			close(h.readyCh)
			return
		}
		go func(elems map[K]chainNode, ready chan struct{}) {
			for _, n := range elems {
				<-n.wait
			}
			close(ready)
		}(h.nodes, h.readyCh)
	})
	return h.readyCh
}

func (h *vectorOperation[K]) Completed() <-chan struct{} {
	// For a vector operation, the completed channel is a combination of all
	// the done channels from its nodes. However, since Complete() closes all
	// the done channels at once, we can create a synthetic completed channel.
	ch := make(chan struct{})
	go func() {
		// Wait for all nodes to be completed
		for _, n := range h.nodes {
			<-n.done
		}
		close(ch)
	}()
	return ch
}

// ShortCircuit checks if all chain elements in the vector are already ready,
// returning true if they are, and false otherwise.
func (h *vectorOperation[K]) shortCircuit() (completed bool) {
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

// Complete attempts to free the memory used by chains that are no longer relevant.
// Chains become irrelevant when all operations associated with them have been
// completed.
func (h *vectorOperation[K]) Complete() {
	h.doneOnce.Do(func() {
		for k, n := range h.nodes {
			close(n.done)
			// Clean up the chain for this key if it is no longer necessary.
			//
			// The map's Delete method guarantees that the chain is removed (and its memory
			// reclaimed) only if the node being marked as complete is the head of the chain for
			// its key.
			//
			// If the node is not the head, it means that there are still operations waiting
			// for this node to complete, so we must keep the chain alive. Otherwise, any
			// Advance() calls to that chain would be detached from those still in progress
			// operations, which violates the causal order.
			h.chains.Delete(k, n)
		}
	})
}
