package causalorder

import (
	"sync"
)

// DependencyGraph enforces causal ordering for operations with multiple
// dependencies expressed as a directed acyclic graph. Operations can depend on
// any number of keys and will only proceed once all their dependencies are
// satisfied. This allows modelling complex dependency relationships while
// maximizing concurrent execution of independent operations.
//
// Operations added via HappensAfter wait for the head operations of all
// specified chain keys to be ready before proceeding. Operations with disjoint
// sets of dependencies can be executed concurrently.
//
// This is the most flexible ordering strategy in the package, allowing
// inter-operation dependencies to be expressed as a graph rather than a linear
// chain.
//
// The DependencyGraph is not safe for concurrent use. It must be used from a
// single goroutine that is responsible for defining the dependency relationships
// between operations. This ensures the causal ordering semantics are well-defined.
//
// The zero-value DependencyGraph is ready to use.
type DependencyGraph[K comparable] struct {
	chains chainMap[K]
}

// HappensAfter returns an Operation that synchronizes after the operations
// associated with the given keys have completed.
func (g *DependencyGraph[K]) HappensAfter(keys ...K) Operation {
	if len(keys) == 0 {
		// If no keys are provided, we can just return an Operation that is ready
		// immediately and does not require any cleaning up.
		return new(loneOperation)
	}

	if len(keys) == 1 {
		// If only one key is provided, we can just use PartialOrder to create a more
		// efficient Operation for that one key.
		po := (*PartialOrder[K])(&g.chains)
		return po.HappensAfter(keys[0])
	}

	// If multiple keys are provided, we need to create a vector operation that will
	// wait for all of them to become ready and signal completion for each of them
	// when it is marked as complete.
	return newVectorOperation(&g.chains, keys)
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

// A graphOperation is an Operation that is ready only when all its associated
// nodes are ready. When completed, it signals completion for each of them when it is marked as
// complete.
//
// It is used to synchronize an operation that depends on multiple operations
// (associated with the given keys) to complete before it can proceed.
type graphOperation[K comparable] struct {
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
	// Waiting for all operations in the vector to complete is done lazily, such that
	// the ready channel is only created when Ready() is called for the first time.
	// See the Ready() method for more details.
	readyLazily sync.Once
	readyCh     chan struct{}
	completed   chan struct{}
}

func newVectorOperation[K comparable](chains *chainMap[K], keys []K) *graphOperation[K] {
	// All chains share the same completed channel, which is closed by Complete().
	// Sharing the same channel avoids allocating a new channel for each node, which
	// would be closed in tandem with the operation's completed channel.
	completed := make(chan struct{})
	dependencies := make(map[K]chainNode, len(keys))
	for _, key := range keys {
		dependencies[key] = chains.Put(key, completed)
	}
	return &graphOperation[K]{
		chains:    chains,
		nodes:     dependencies,
		completed: completed,
	}
}

// Ready returns a channel that closes when all operations in the vector are
// ready to proceed. It defers creating that channel for as long as possible to
// increase the chance of short-circuiting the concurrent waiting goroutine,
// thereby reducing memory usage and improving resource usage.
func (o *graphOperation[K]) Ready() <-chan struct{} {
	o.readyLazily.Do(func() {
		o.readyCh = make(chan struct{})
		if o.shortCircuit() {
			// If all vector elements are already ready, we can close the ready channel
			// immediately, without waiting for all those elements to become ready in a
			// separate goroutine.
			close(o.readyCh)
			return
		}

		// If not all dependent chains are ready, we must wait for them in a goroutine to
		// avoid blocking the caller. This goroutine lives until ALL current heads of the
		// chains are completed. The package requires callers to ALWAYS call Complete()
		// on any Operation, so we can safely assume that the goroutine will eventually
		// terminate.
		go func(nodes map[K]chainNode, ready chan struct{}) {
			for _, n := range nodes {
				<-n.wait
			}
			close(ready)
		}(o.nodes, o.readyCh)
	})
	return o.readyCh
}

func (o *graphOperation[K]) Completed() <-chan struct{} {
	return o.completed
	// For a vector operation, the completed channel is a combination of all
	// the done channels from its nodes. However, since Complete() closes all
	// the done channels at once, we can create a synthetic completed channel.
	//ch := make(chan struct{})
	//go func() {
	//	// Wait for all nodes to be completed
	//	for _, n := range o.nodes {
	//		<-n.done
	//	}
	//	close(ch)
	//}()
	//return ch
}

// ShortCircuit checks if all chain elements in the vector are already ready,
// returning true if they are, and false otherwise.
func (o *graphOperation[K]) shortCircuit() (completed bool) {
	for _, n := range o.nodes {
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

// Complete marks this operation as completed for users and marks all nodes
//
// attempts to free the memory used by chains that are no longer relevant.
// Chains become irrelevant when all operations associated with them have been
// completed.
func (o *graphOperation[K]) Complete() {
	o.doneOnce.Do(func() {
		// We must first close the completed channel before attempting to reclaim memory
		// from the map of chains. Otherwise, we risk racing between the goroutine that
		// called this method and another goroutine that is advancing chains (by calling
		// HappensAfter).
		//
		// More specifically, if the map reclaims a chain before the completed channel is
		// closed, then another goroutine may attempt to advance that reclaimed chain,
		// which will make it immediately ready. This, in turn, will violate the causal
		// guarantee of operations in the same DependencyGraph because this race may
		// cause the newer operation to appear Ready before the older one has been marked
		// as Completed.
		//
		// On the flip side, delaying the reclamation of chains by closing the completed
		// channel first allows for further operations to be added to chains. This is
		// actually more efficient than deleting a chain which would soon be recreated.
		close(o.completed)
		for k, n := range o.nodes {
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
			o.chains.Delete(k, n)
		}
	})
}
