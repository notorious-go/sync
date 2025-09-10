// Package ordering provides synchronization primitives for managing the causal
// ordering of concurrent operations. It offers ordering strategies that allow
// developers to express different types of dependencies between operations
// while maximizing concurrent execution where possible.
//
// The package is organized into sub-packages, each implementing a different
// ordering strategy:
//
//   - totalorder: Enforces strict sequential ordering of all operations
//   - partialorder: Enforces sequential ordering per key, allowing concurrent execution across keys
//
// # Operation Interface
//
// The [Operation] interface is the core abstraction that all ordering strategies
// build upon. It provides three methods:
//
//   - Ready(): Returns a channel that closes when the operation can begin
//   - Complete(): Marks the operation as finished, unblocking dependent operations
//   - Completed(): Returns a channel that closes when Complete() is called
//
// # Usage Patterns
//
// All ordering types follow a similar usage pattern:
//
//	var order OrderingType
//	op := order.Schedule(...)
//	go func() {
//	    defer op.Complete()
//	    <-op.Ready()
//	    // perform work
//	}()
//
// The Await helper function simplifies the common pattern:
//
//	done := ordering.Await(op)
//	defer done()
//	// ... perform operation ...
//
// It is also common for workers to abort operations; most commonly cancelled
// using context:
//
//	select {
//	case <-op.Ready():
//	    defer op.Complete()
//	    // perform work
//	    return nil
//	case <-ctx.Done():
//	    op.Complete() // Important: still call Complete
//	    return ctx.Err()
//	}
//
// # Choosing an Ordering Strategy
//
// Use the totalorder package when the source of tasks is a totally ordered
// stream, wherein each operation must happen strictly after the previous one.
//
//	var order totalorder.TotalOrder
//	for _, task := range tasks {
//	    op := order.HappensNext()
//	    go processTask(op, task)
//	}
//
// Use the partialorder package when the source contains partially ordered
// events, wherein operations must be ordered within groups but can run
// concurrently across groups:
//
//	var order partialorder.PartialOrder[string]
//	for _, event := range events {
//	    op := order.HappensAfter(event.UserID)
//	    go processUserEvent(op, event)
//	}
package ordering
