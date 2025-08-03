package causalgroup

import (
	"fmt"
	"sync"

	"github.com/notorious-go/sync/causalorder"
	"github.com/notorious-go/sync/semaphore"
)

// A group is a collection of goroutines that can be spawned with a limit on
// the number of active goroutines.
//
// Each goroutine blocks until its associated causal dependencies are ready,
// ensuring that each concurrent execution respects the specific causal order of
// operations.
//
// It is designed to be used for in combination with specific causal orderings by
// embedding it in Queue, Topic, and Matrix.
//
// A zero group is valid and has no limit on the number of active goroutines.
type group struct {
	wg  sync.WaitGroup
	sem semaphore.Semaphore
}

func (g *group) done(op causalorder.Operation) {
	op.Complete()
	g.sem.Release()
	g.wg.Done()
}

// Wait blocks until all function calls from the Go and TryGo methods have
// returned.
func (g *group) Wait() {
	g.wg.Wait()
}

// Spawn calls the given function in a new goroutine. It blocks until the new
// goroutine can be added without the number of active goroutines in the group
// exceeding the configured limit.
//
// The new goroutine will block before calling f until the given causal operation
// is ready, ensuring that it respects the specific causal order of operations.
func (g *group) spawn(op causalorder.Operation, f func()) {
	g.sem.Acquire()
	g.wg.Add(1)
	go func() {
		defer g.done(op)
		<-op.Ready()
		f()
	}()
}

// SetLimit limits the number of active goroutines in this group to at most n. A
// negative value indicates no limit. A zero value will block any further calls
// to Go or TryGo.
//
// Any further call to the Go method will block until it can add an active
// goroutine without exceeding the configured limit.
//
// The limit must not be modified while any goroutines in the group are active.
func (g *group) SetLimit(n int) {
	if len(g.sem) != 0 {
		panic(fmt.Errorf("causalgroup: modify limit while %v goroutines in the group are still active", len(g.sem)))
	}
	g.sem = semaphore.NewSemaphore(n)
}

// A Matrix is a collection of goroutines working on tasks that maintain a
// complex order of execution based on multiple chains. Each task blocks until
// all previously submitted tasks with any of the same chain keys have completed.
//
// Unlike Topic, which blocks only on tasks with the exact same partition key,
// Matrix blocks on tasks that share any common key, allowing for more complex
// dependency relationships while still permitting concurrent execution of
// completely independent operations.
//
// A zero Matrix is valid and has no limit on the number of active goroutines.
type Matrix[K comparable] struct {
	group
	ordering causalorder.DependencyGraph[K]
}

// Go calls the given function in a new goroutine. It blocks until the new
// goroutine can be added without the number of active goroutines in the group
// exceeding the configured limit.
//
// The new goroutine will block before calling f until all previously submitted
// tasks that share any of the given chain keys have completed. Tasks that share
// no common keys can execute concurrently.
func (m *Matrix[K]) Go(keys []K, f func()) {
	op := m.ordering.HappensAfter(keys...)
	m.spawn(op, f)
}

// A Topic is a collection of goroutines working on tasks that maintain a partial
// order of execution based on their partition key. Each task blocks until all
// previously submitted tasks with the same partition key have completed.
//
// Unlike Queue, which blocks until all previous tasks are complete, Topic only
// blocks on tasks within the same partition, allowing unrelated operations to
// proceed without blocking on each other.
//
// A zero Topic is valid and has no limit on the number of active goroutines.
type Topic[K comparable] struct {
	group
	ordering causalorder.PartialOrder[K]
}

// Go calls the given function in a new goroutine. It blocks until the new
// goroutine can be added without the number of active goroutines in the group
// exceeding the configured limit.
//
// The new goroutine will block before calling f until all previously submitted
// tasks with the same partition key have completed. Tasks with different
// partition keys do not block on each other.
func (t *Topic[K]) Go(partition K, f func()) {
	op := t.ordering.HappensAfter(partition)
	t.spawn(op, f)
}

// A Queue is a collection of goroutines working on tasks that must maintain a
// strict total order. Each task blocks until all previously submitted tasks have
// completed their execution.
//
// A zero Queue is valid and has no limit on the number of active goroutines.
type Queue struct {
	group
	ordering causalorder.TotalOrder
}

// Go calls the given function in a new goroutine. It blocks until the new
// goroutine can be added without the number of active goroutines in the group
// exceeding the configured limit.
//
// The new goroutine will block before calling f until all previously submitted
// tasks have completed, ensuring a strict sequential execution order.
func (q *Queue) Go(f func()) {
	op := q.ordering.HappensAfter()
	q.spawn(op, f)
}
