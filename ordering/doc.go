// Package ordering provides synchronization primitives for managing the causal
// ordering of concurrent operations.
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
package ordering
