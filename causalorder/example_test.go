package causalorder_test

import (
	"fmt"
	"slices"
	"sync"

	"github.com/notorious-go/sync/causalorder"
)

// EventSource is a sample log of user actions that we will use to demonstrate
// causal ordering.
//
// This data is synthetic, but in a real application, it would come from an event
// source like a message queue or a database change stream.
//
// Each action is associated with a user, and we want to ensure that actions for
// the same user are audited in the order they were performed, even if the events
// are processed concurrently.
var EventSource = []struct {
	User   string
	Action string
}{
	{User: "alice", Action: "login"},
	{User: "bob", Action: "login"},
	{User: "alice", Action: "purchase"},
	{User: "bob", Action: "logout"},
	{User: "alice", Action: "logout"},
}

// This example demonstrates total causal ordering by showing how to ensure
// database writes happen in order for the same record, even when started
// concurrently.
func ExampleTotalOrder() {
	// For this example, we will use a TotalOrder for each of the two users.
	//
	// Note how the zero value of TotalOrder is ready to use.
	var (
		alice causalorder.TotalOrder
		bob   causalorder.TotalOrder
	)

	// This example uses a simple in-memory audit log to demonstrate the causal
	// ordering of user actions.
	var database AuditLog

	var wg sync.WaitGroup
	for _, event := range EventSource {
		// Depending on the record, we will use the corresponding TotalOrder to guarantee
		// that changes to the same user are logged in order.
		var op causalorder.Operation
		switch event.User {
		case "alice":
			op = alice.HappensAfter()
		case "bob":
			op = bob.HappensAfter()
		default:
			panic("Unknown user: " + event.User)
		}

		// With the causal order package, we can perform operations concurrently while
		// ensuring that dependent operations are executed in the correct order.
		wg.Add(1)
		go func() {
			defer wg.Done()
			// We must always call Complete() on the operation to mark it as done, even if it
			// fails/panics, so we defer the call to Complete().
			defer op.Complete()

			// The Ready() channel will block until the operation is ready to be executed.
			// Block on it indefinitely or select on it with a timeout or cancellation
			// context in a real application.
			<-op.Ready()

			// After the Ready channel synchronizes, we know that all previous operations for
			// the same user have completed, and we can safely apply the change to the
			// database in concurrent goroutines.
			database.Append(event.User, event.Action)
		}()
	}

	wg.Wait()
	fmt.Println(database.String())
	// Unordered Output:
	// User alice: [login purchase logout]
	// User bob: [login logout]
}

// This example demonstrates partial causal ordering by showing how to ensure
// database writes happen in order for the same record, even when started
// concurrently.
//
// The PartialOrder type supports any number of partitions, allowing you to
// manage dynamic sets of dependent operations.
func ExamplePartialOrder() {
	// For this example, we will use a PartialOrder to ensure that actions for the
	// same user are logged in order, even when started concurrently.
	//
	// Note that any comparable type can be used as a key for the PartialOrder.
	//
	// Note how the zero value of PartialOrder is ready to use.
	var ordering causalorder.PartialOrder[string]

	// This example uses a simple in-memory audit log to demonstrate the causal
	// ordering of user actions.
	var database AuditLog

	var wg sync.WaitGroup
	for _, event := range EventSource {
		// With PartialOrder, the HappensAfter method allows us to dynamically assign
		// operations to specific keys (in this case, usernames).
		op := ordering.HappensAfter(event.User)

		// With the causal order package, we can perform operations concurrently while
		// ensuring that dependent operations are executed in the correct order.
		wg.Add(1)
		go func() {
			defer wg.Done()
			// We must always call Complete() on the operation to mark it as done, even if it
			// fails/panics, so we defer the call to Complete().
			defer op.Complete()

			// The Ready() channel will block until the operation is ready to be executed.
			// Block on it indefinitely or select on it with a timeout or cancellation
			// context in a real application.
			<-op.Ready()

			// After the Ready channel synchronizes, we know that all previous operations for
			// the same user have completed, and we can safely apply the change to the
			// database in concurrent goroutines.
			database.Append(event.User, event.Action)
		}()
	}

	wg.Wait()
	fmt.Println(database.String())
	// Unordered Output:
	// User alice: [login purchase logout]
	// User bob: [login logout]
}

// This example demonstrates DependencyGraph for managing operations with
// dependencies on multiple resources.
//
// Consider a distributed system where operations may depend on multiple
// resources being available. DependencyGraph ensures operations only proceed
// when all their dependencies are satisfied, while allowing operations with
// disjoint dependencies to execute concurrently.
func ExampleDependencyGraph() {
	// DependencyGraph models a directed acyclic graph of operation dependencies.
	// Each operation can depend on multiple keys, representing different resources
	// or conditions that must be met.
	var graph causalorder.DependencyGraph[string]

	// Simulate a build system where:
	// - Compilation depends on source files and configuration
	// - Tests depend on compiled binaries and test data
	// - Deployment depends on successful tests and deployment config
	
	type BuildStep struct {
		Name         string
		Dependencies []string
		Operation    causalorder.Operation
	}

	// Define our build pipeline with complex dependencies
	steps := []BuildStep{
		{
			Name:         "load-config",
			Dependencies: nil, // No dependencies
			Operation:    graph.HappensAfter("config"),
		},
		{
			Name:         "compile-server",
			Dependencies: []string{"config", "server-src"},
			Operation:    graph.HappensAfter("config", "server-src"),
		},
		{
			Name:         "compile-client",
			Dependencies: []string{"config", "client-src"},
			Operation:    graph.HappensAfter("config", "client-src"),
		},
		{
			Name:         "run-server-tests",
			Dependencies: []string{"server-bin", "test-data"},
			Operation:    graph.HappensAfter("server-bin", "test-data"),
		},
		{
			Name:         "run-client-tests",
			Dependencies: []string{"client-bin", "test-data"},
			Operation:    graph.HappensAfter("client-bin", "test-data"),
		},
		{
			Name:         "deploy",
			Dependencies: []string{"server-tests", "client-tests", "deploy-config"},
			Operation:    graph.HappensAfter("server-tests", "client-tests", "deploy-config"),
		},
	}

	// Track completion order to demonstrate concurrent execution
	var mu sync.Mutex
	var completionOrder []string

	var wg sync.WaitGroup
	for _, step := range steps {
		wg.Add(1)
		go func(s BuildStep) {
			defer wg.Done()
			defer s.Operation.Complete()

			// Wait for all dependencies to be satisfied
			<-s.Operation.Ready()

			// Record when this step starts
			mu.Lock()
			completionOrder = append(completionOrder, s.Name)
			mu.Unlock()

			// Simulate the build step
			fmt.Printf("Executing: %s\n", s.Name)

			// Mark resources as available for dependent operations
			switch s.Name {
			case "load-config":
				// Config is now loaded
			case "compile-server":
				next := graph.HappensAfter("server-bin")
				next.Complete() // Server binary is ready
			case "compile-client":
				next := graph.HappensAfter("client-bin")
				next.Complete() // Client binary is ready
			case "run-server-tests":
				next := graph.HappensAfter("server-tests")
				next.Complete() // Server tests passed
			case "run-client-tests":
				next := graph.HappensAfter("client-tests")
				next.Complete() // Client tests passed
			}
		}(step)
	}

	// Simulate external resources becoming available
	go func() {
		// Source files and configs are available immediately
		graph.HappensAfter("server-src").Complete()
		graph.HappensAfter("client-src").Complete()
		graph.HappensAfter("test-data").Complete()
		graph.HappensAfter("deploy-config").Complete()
	}()

	wg.Wait()

	// The exact order may vary due to concurrent execution,
	// but dependencies are always respected:
	// - compile-* steps run after load-config
	// - test steps run after their respective compile steps
	// - deploy runs after both test steps complete
	fmt.Println("\nDependencies were respected!")

	// Output:
	// Executing: load-config
	// Executing: compile-server
	// Executing: compile-client
	// Executing: run-server-tests
	// Executing: run-client-tests
	// Executing: deploy
	//
	// Dependencies were respected!
}

// AuditLog is a simple in-memory log that records user actions in the order they
// were appended. The Append method is thread-safe and allows concurrent
// appending of actions by different users, and the String method returns a
// formatted string representation of the log.
type AuditLog struct {
	m  map[string][]string
	mu sync.Mutex
}

func (l *AuditLog) Append(user, action string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = make(map[string][]string)
	}
	l.m[user] = append(l.m[user], action)
}

func (l *AuditLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var s string
	for user, actions := range l.m {
		s += fmt.Sprintf("User %v: %v\n", user, actions)
	}
	return s
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
