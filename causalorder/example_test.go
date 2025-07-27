package causalorder_test

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/notorious-go/sync/causalorder"
)

// This example demonstrates total causal ordering by showing how to ensure
// database writes happen in order for the same record, even when started
// concurrently.
func ExampleTotalOrder() {
	// For this example, we will use a TotalOrder for each of the two database
	// records.
	//
	// Note how the zero value of TotalOrder is ready to use.
	var (
		record1 causalorder.TotalOrder
		record2 causalorder.TotalOrder
	)

	// For this example, we define a Change Data Capture (CDC) type that represents a
	// change to a database record.
	type CDC struct {
		Record int
	}

	// For each record, we will create three operations that must be executed in
	// order, even though they are scheduled for execution concurrently.
	record1changes := [3]CDC{}

	// Output:
	// Writing record 1
	// Writing record 2
	// Writing record 3
}

// ExamplePartialOrder shows how to maintain order within each partition
// while allowing different partitions to proceed independently.
// The zero value is ready to use.
func ExamplePartialOrder() {
	var order causalorder.PartialOrder[string]

	// Start all operations
	opA1 := order.HappensAfter("alice")
	opA2 := order.HappensAfter("alice")
	opA3 := order.HappensAfter("alice")
	opB1 := order.HappensAfter("bob")
	opB2 := order.HappensAfter("bob")

	// Check which operations are ready
	select {
	case <-opA1.Ready():
		fmt.Println("alice: login")
		opA1.Complete()
	default:
		fmt.Println("alice login blocked")
	}

	select {
	case <-opB1.Ready():
		fmt.Println("bob: login")
		opB1.Complete()
	default:
		fmt.Println("bob login blocked")
	}

	select {
	case <-opA2.Ready():
		fmt.Println("alice: purchase")
		opA2.Complete()
	default:
		fmt.Println("alice purchase blocked")
	}

	select {
	case <-opB2.Ready():
		fmt.Println("bob: logout")
		opB2.Complete()
	default:
		fmt.Println("bob logout blocked")
	}

	select {
	case <-opA3.Ready():
		fmt.Println("alice: logout")
		opA3.Complete()
	default:
		fmt.Println("alice logout blocked")
	}

	// Output:
	// alice: login
	// bob: login
	// alice: purchase
	// bob: logout
	// alice: logout
}

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
	// For this example, we will use a TotalOrder to create sequential operations.
	var order causalorder.TotalOrder
	op1 := order.HappensAfter()
	op2 := order.HappensAfter()

	// We will define helper functions to check the status of an operation.
	checkStatus := func(op causalorder.Operation) (status string) {
		if Completed(op) {
			// Technically, an operation can be completed without being ready, but this
			// should not happen in well-behaved code.
			if !Ready(op) {
				return "completed (detached)"
			}
			return "completed"
		}
		// An operation that is pending until it is completed. While pending, it can be
		// either ready to execute or blocking on dependent operations.
		if Ready(op) {
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

// Ready checks whether the given operation is ready to be executed.
//
// If the given operation is not ready, we say it is blocking, meaning it is
// waiting for some other operations to complete before it can be executed.
func Ready(op causalorder.Operation) (ready bool) {
	select {
	case <-op.Ready():
		return true
	default:
		return false
	}
}

// Completed checks whether the given operation has been completed.
//
// If the given operation is not completed, we say it is pending, meaning it is
// either ready to be executed or blocking on some other operations to complete
// before it can be executed.
func Completed(op causalorder.Operation) (completed bool) {
	select {
	case <-op.Completed():
		return true
	default:
		return false
	}
}

// This example demonstrates the use of the Await convenience function for simple
// interactions with operations.
//
// Await is suitable for cases where you want to block until an operation is
// ready and then execute it, without considering timeouts or cancellation.
func ExampleAwait() {
	// For this example, we will use a TotalOrder to create sequential operations.
	var order causalorder.TotalOrder

	// For this example, we will define a Task type that represents a causally
	// ordered operation.
	type Task struct {
		Id int
		causalorder.Operation
	}

	// First, let's create three tasks that must execute in sequence.
	var tasks []Task
	for i := range 3 {
		// Calling HappensAfter() returns an causalorder.Operation that is ready to be
		// executed after all previous operations in our exemplar total order have
		// completed.
		//
		// Note how the operations are ordered by the order we call HappensAfter(), so it
		// is never called concurrently. Once the order is established, we can execute
		// the operations concurrently and still ensure they happen in the correct order.
		t := Task{Id: i + 1, Operation: order.HappensAfter()}
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
			done := causalorder.Await(t)
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
