package totalorder

import (
	"fmt"
	"sync"

	"github.com/notorious-go/sync/ordering"
	"github.com/notorious-go/sync/semaphore"
)

// A Queue is a collection of goroutines working on tasks that must maintain a
// strict total order. Each task blocks until all previously submitted tasks have
// completed their execution.
//
// A zero Queue is valid and has no limit on the number of active goroutines.
type Queue struct {
	wg       sync.WaitGroup
	sem      semaphore.Semaphore
	ordering TotalOrder
}

// Go calls the given function in a new goroutine. It blocks until the new
// goroutine can be added without the number of active goroutines in the group
// exceeding the configured limit.
//
// The new goroutine will block before calling f until all previously submitted
// tasks have completed, ensuring a strict sequential execution order.
func (q *Queue) Go(f func()) {
	op := q.ordering.HappensNext()
	q.sem.Acquire()
	q.wg.Add(1)
	go func() {
		defer q.done(op)
		<-op.Ready()
		f()
	}()
}

func (q *Queue) done(op ordering.Operation) {
	op.Complete()
	q.sem.Release()
	q.wg.Done()
}

// Wait blocks until all function calls from the Go method have returned.
func (q *Queue) Wait() {
	q.wg.Wait()
}

// SetLimit limits the number of active goroutines in this group to at most n. A
// negative value indicates no limit. A zero value will block any further calls
// to Go.
//
// The limit must not be modified while any goroutines in the group are active.
func (q *Queue) SetLimit(n int) {
	if len(q.sem) != 0 {
		panic(fmt.Errorf("queue: modify limit while %v goroutines in the group are still active", len(q.sem)))
	}
	q.sem = semaphore.New(n)
}
