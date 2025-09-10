package ordering_test

import (
	"fmt"
	"slices"
	"sync"

	"github.com/notorious-go/sync/ordering"
	"github.com/notorious-go/sync/ordering/totalorder"
)

// This example demonstrates how to determine the current status of an Operation
// using select statements with the Ready and Completed channels.
//
// Typically, an Operation transitions through several states during its lifecycle:
//
//   - Initially, it is blocking, waiting for dependencies to complete.
//   - Then, it becomes ready to execute when all dependencies are satisfied. After
//     execution starts, it is in a pending state until it completes.
//   - Finally, it is completed when the operation has finished executing.
func ExampleOperation_status() {
	// For this example, we will use a TotalOrder to create sequential operations,
	// wherein op2 happens after op1.
	var order totalorder.TotalOrder
	op1 := order.HappensNext()
	op2 := order.HappensNext()

	// We will define helper functions to check the status of an operation.
	checkStatus := func(op ordering.Operation) (status string) {
		if ordering.Completed(op) {
			// Technically, an operation can be completed without being ready, but this
			// should not happen in well-behaved code.
			if !ordering.Ready(op) {
				return "completed (detached)"
			}
			return "completed"
		}
		// An operation that is pending until it is completed. While pending, it can be
		// either ready to execute or blocking on dependent operations.
		if ordering.Ready(op) {
			return "pending (ready)"
		}
		return "pending (blocking)"
	}

	// Initial state: op1 is ready, op2 is blocking.
	fmt.Println("op1:", checkStatus(op1))
	fmt.Println("op2:", checkStatus(op2))

	// After op1 completes, op2 becomes ready.
	op1.Complete()
	fmt.Println("op1:", checkStatus(op1))
	fmt.Println("op2:", checkStatus(op2))

	// After op2 completes, both operations are completed.
	op2.Complete()
	fmt.Println("op1:", checkStatus(op1))
	fmt.Println("op2:", checkStatus(op2))

	// Output:
	// op1: pending (ready)
	// op2: pending (blocking)
	// op1: completed
	// op2: pending (ready)
	// op1: completed
	// op2: completed
}

// This example demonstrates the use of the Await convenience function for simple
// interactions with operations.
//
// Await is suitable for cases where you want to block until an operation is
// ready and then execute it, without considering timeouts or cancellation.
func ExampleAwait() {
	// For this example, we will use a TotalOrder to create sequential operations.
	var order totalorder.TotalOrder

	// For this example, we will define a Task type that represents a causally
	// ordered operation.
	type Task struct {
		Id int
		ordering.Operation
	}

	// First, let's create three tasks that must execute in sequence.
	var tasks []Task
	for i := range 3 {
		// Calling HappensNext() returns an ordering.Operation that is ready to be
		// executed after all previous operations in our exemplar total order have
		// completed.
		//
		// Note how the operations are ordered by the order we call HappensNext(), so it
		// is never called concurrently. Once the order is established, we can execute
		// the operations concurrently and still ensure they happen in the correct order.
		t := Task{Id: i + 1, Operation: order.HappensNext()}
		tasks = append(tasks, t)
	}

	// Though the total-order guarantees that operations will be executed in
	// sequence, we still want to demonstrate the concurrent execution pattern.
	//
	// To prove the ordering guarantees of the total order, we will spawn tasks in
	// reverse order, but they will still execute in the correct order due to the
	// causality constraints.
	var wg sync.WaitGroup
	for _, t := range slices.Backward(tasks) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Await blocks until ready and returns a done function, which should be deferred
			// to ensure the operation is marked as complete no matter what (yes, even panics).
			done := ordering.Await(t)
			defer done() // Always mark the operation as complete!

			// -- simulate work --
			fmt.Println("Executing operation", t.Id)
		}()
	}
	wg.Wait()

	// Output:
	// Executing operation 1
	// Executing operation 2
	// Executing operation 3
}
