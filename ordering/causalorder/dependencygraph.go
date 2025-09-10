package causalorder

import (
	"fmt"
	"sync"

	"github.com/notorious-go/sync/ordering"
	"github.com/notorious-go/sync/semaphore"
)

// A DependencyGraph is a collection of goroutines working on tasks that maintain a
// complex order of execution based on multiple chains. Each task blocks until
// all previously submitted tasks with any of the same chain keys have completed.
//
// Unlike Topic, which blocks only on tasks with the exact same partition key,
// DependencyGraph blocks on tasks that share any common key, allowing for more complex
// dependency relationships while still permitting concurrent execution of
// completely independent operations.
//
// A zero DependencyGraph is valid and has no limit on the number of active goroutines.
type DependencyGraph[K comparable] struct {
	wg       sync.WaitGroup
	sem      semaphore.Semaphore
	ordering CausalOrder[K]
}

// Go calls the given function in a new goroutine. It blocks until the new
// goroutine can be added without the number of active goroutines in the group
// exceeding the configured limit.
//
// The new goroutine will block before calling f until all previously submitted
// tasks that share any of the given chain keys have completed. Tasks that share
// no common keys can execute concurrently.
func (d *DependencyGraph[K]) Go(keys []K, f func()) {
	op := d.ordering.HappensAfter(keys...)
	d.sem.Acquire()
	d.wg.Add(1)
	go func() {
		defer d.done(op)
		<-op.Ready()
		f()
	}()
}

func (d *DependencyGraph[K]) done(op ordering.Operation) {
	op.Complete()
	d.sem.Release()
	d.wg.Done()
}

// Wait blocks until all function calls from the Go method have returned.
func (d *DependencyGraph[K]) Wait() {
	d.wg.Wait()
}

// SetLimit limits the number of active goroutines in this group to at most n. A
// negative value indicates no limit. A zero value will block any further calls
// to Go.
//
// The limit must not be modified while any goroutines in the group are active.
func (d *DependencyGraph[K]) SetLimit(n int) {
	if len(d.sem) != 0 {
		panic(fmt.Errorf("dependencygraph: modify limit while %v goroutines in the group are still active", len(d.sem)))
	}
	d.sem = semaphore.New(n)
}
