package partialorder

import (
	"fmt"
	"sync"

	"github.com/notorious-go/sync/ordering"
	"github.com/notorious-go/sync/semaphore"
)

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
	wg       sync.WaitGroup
	sem      semaphore.Semaphore
	ordering PartialOrder[K]
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
	t.sem.Acquire()
	t.wg.Add(1)
	go func() {
		defer t.done(op)
		<-op.Ready()
		f()
	}()
}

func (t *Topic[K]) done(op ordering.Operation) {
	op.Complete()
	t.sem.Release()
	t.wg.Done()
}

// Wait blocks until all function calls from the Go method have returned.
func (t *Topic[K]) Wait() {
	t.wg.Wait()
}

// SetLimit limits the number of active goroutines in this group to at most n. A
// negative value indicates no limit. A zero value will block any further calls
// to Go.
//
// The limit must not be modified while any goroutines in the group are active.
func (t *Topic[K]) SetLimit(n int) {
	if len(t.sem) != 0 {
		panic(fmt.Errorf("topic: modify limit while %v goroutines in the group are still active", len(t.sem)))
	}
	t.sem = semaphore.New(n)
}
