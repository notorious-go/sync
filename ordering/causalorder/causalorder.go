package causalorder

import (
	"sync"

	"github.com/notorious-go/sync/ordering"
)

// chain represents a linked list of operations for a single key
type chain struct {
	head chan struct{}
}

// init ensures that the chain is ready to use
func (c *chain) init() {
	if c.head == nil {
		c.head = make(chan struct{})
		close(c.head)
	}
}

// put sets the new head of the chain and returns the wait channel
func (c *chain) put(done chan struct{}) <-chan struct{} {
	c.init()
	wait := c.head
	c.head = done
	return wait
}

// isHead checks if the given done channel is the current head of the chain
func (c *chain) isHead(done chan struct{}) bool {
	return c.head == done
}

// CausalOrder enforces causal ordering for operations with multiple
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
// The CausalOrder is not safe for concurrent use. It must be used from a
// single goroutine that is responsible for defining the dependency relationships
// between operations. This ensures the causal ordering semantics are well-defined.
//
// The zero-value CausalOrder is ready to use.
type CausalOrder[K comparable] struct {
	// Makes the zero-value CausalOrder ready to use and concurrent-safe.
	initOnce sync.Once
	// We must protect the map of chains against concurrent access.
	chains chan map[K]*chain
}

// acquire gets exclusive access to the chains map
func (o *CausalOrder[K]) acquire() (chains map[K]*chain, release func()) {
	o.initOnce.Do(func() {
		o.chains = make(chan map[K]*chain, 1)
		o.chains <- make(map[K]*chain)
	})
	chains = <-o.chains
	release = func() { o.chains <- chains }
	return chains, release
}

// put creates a new head for the chain associated with the given key using the
// given done channel, and returns the wait channel
func (o *CausalOrder[K]) put(key K, done chan struct{}) <-chan struct{} {
	chains, release := o.acquire()
	defer release()
	if _, exists := chains[key]; !exists {
		chains[key] = new(chain)
	}
	return chains[key].put(done)
}

// delete removes the chain for the given key if the done channel is the head
func (o *CausalOrder[K]) delete(key K, done chan struct{}) {
	chains, release := o.acquire()
	defer release()
	if c, ok := chains[key]; ok && c.isHead(done) {
		delete(chains, key)
	}
}

// HappensAfter returns an Operation that synchronizes after the operations
// associated with the given keys have completed.
func (o *CausalOrder[K]) HappensAfter(keys ...K) ordering.Operation {
	if len(keys) == 0 {
		// If no keys are provided, we can just return an Operation that is ready
		// immediately and does not require any cleaning up.
		return new(loneOperation)
	}

	if len(keys) == 1 {
		// If only one key is provided, we can create a simpler operation
		return o.singleKeyOperation(keys[0])
	}

	// If multiple keys are provided, we need to create a causal operation that will
	// wait for all of them to become ready and signal completion for each of them
	// when it is marked as complete.
	return o.multiKeyOperation(keys)
}

// singleKeyOperation creates an operation for a single key dependency
func (o *CausalOrder[K]) singleKeyOperation(key K) ordering.Operation {
	chains, release := o.acquire()
	defer release()

	if _, exists := chains[key]; !exists {
		chains[key] = new(chain)
	}

	done := make(chan struct{})
	wait := chains[key].put(done)

	return &singleKeyOp[K]{
		wait:  wait,
		done:  done,
		key:   key,
		order: o,
	}
}

// multiKeyOperation creates an operation for multiple key dependencies
func (o *CausalOrder[K]) multiKeyOperation(keys []K) ordering.Operation {
	// All chains share the same completed channel, which is closed by Complete().
	// Sharing the same channel avoids allocating a new channel for each node.
	completed := make(chan struct{})
	dependencies := make(map[K]<-chan struct{}, len(keys))

	for _, key := range keys {
		dependencies[key] = o.put(key, completed)
	}

	return &multiKeyOp[K]{
		order:        o,
		dependencies: dependencies,
		completed:    completed,
	}
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
		o.initOnce()
		close(o.completed)
	})
}

// singleKeyOp is an operation with a single key dependency
type singleKeyOp[K comparable] struct {
	wait     <-chan struct{}
	done     chan struct{}
	doneOnce sync.Once
	key      K
	order    *CausalOrder[K]
}

func (o *singleKeyOp[K]) Ready() <-chan struct{} {
	return o.wait
}

func (o *singleKeyOp[K]) Completed() <-chan struct{} {
	return o.done
}

func (o *singleKeyOp[K]) Complete() {
	o.doneOnce.Do(func() {
		close(o.done)
		o.order.delete(o.key, o.done)
	})
}

// multiKeyOp is an operation with multiple key dependencies
type multiKeyOp[K comparable] struct {
	order        *CausalOrder[K]
	dependencies map[K]<-chan struct{}
	completed    chan struct{}
	doneOnce     sync.Once
	readyLazily  sync.Once
	readyCh      chan struct{}
}

// Ready returns a channel that closes when all operations in the vector are
// ready to proceed. It defers creating that channel for as long as possible to
// increase the chance of short-circuiting the concurrent waiting goroutine,
// thereby reducing memory usage and improving resource usage.
func (o *multiKeyOp[K]) Ready() <-chan struct{} {
	o.readyLazily.Do(func() {
		o.readyCh = make(chan struct{})
		if o.shortCircuit() {
			// If all dependencies are already ready, we can close the ready channel
			// immediately, without waiting for them in a separate goroutine.
			close(o.readyCh)
			return
		}

		// If not all dependencies are ready, we must wait for them in a goroutine to
		// avoid blocking the caller.
		go func() {
			for _, wait := range o.dependencies {
				<-wait
			}
			close(o.readyCh)
		}()
	})
	return o.readyCh
}

func (o *multiKeyOp[K]) Completed() <-chan struct{} {
	return o.completed
}

// shortCircuit checks if all dependencies are already ready
func (o *multiKeyOp[K]) shortCircuit() bool {
	for _, wait := range o.dependencies {
		select {
		case <-wait:
		default:
			// If even a single dependency is not immediately ready, then the short-circuiting
			// is not possible, and we need to wait for all of them to become ready.
			return false
		}
	}
	return true
}

// Complete marks this operation as completed and cleans up chains
func (o *multiKeyOp[K]) Complete() {
	o.doneOnce.Do(func() {
		// We must first close the completed channel before attempting to reclaim memory
		// from the map of chains. Otherwise, we risk racing between the goroutine that
		// called this method and another goroutine that is advancing chains.
		close(o.completed)

		// Clean up each chain if it's no longer necessary
		for key := range o.dependencies {
			o.order.delete(key, o.completed)
		}
	})
}
