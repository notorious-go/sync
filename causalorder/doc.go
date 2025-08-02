// Package causalorder provides synchronization primitives for enforcing
// causal ordering of operations in concurrent programs.
//
// This package addresses the challenge of processing events or operations where
// some must complete before others can begin, while maximizing concurrent
// execution of independent operations. Many concurrent systems need to respect
// causal dependencies while processing operations in parallel to achieve high
// throughput. The standard sync.WaitGroup can coordinate concurrent operations
// but provides no ordering guarantees. This package enables operations to
// participate in multiple independent execution flows while ensuring all causal
// dependencies are respected.
//
// # Ordering Strategies
//
// The package offers three main ordering strategies:
//
//   - [TotalOrder]: Ensures all operations execute in a strict sequential order.
//     Operations are processed one at a time in the exact order they were added.
//
//   - [PartialOrder]: Ensures operations with the same key execute sequentially,
//     while operations with different keys can execute concurrently. This is
//     useful for scenarios like processing events grouped by an identifier.
//
//   - [VectorOrder]: Ensures operations execute in a causal order based on
//     multiple keys, allowing concurrent execution of operations that depend on
//     different sets of keys. An operation waits for all specified keys to be
//     ready before proceeding.
//
// # Operation API
//
// All ordering strategies use an Operation-based API where HappensAfter returns
// an [Operation] that must be completed before dependent operations can proceed.
//
// The [Operation] interface provides a channel-based API for flexible
// synchronization:
//   - Block until ready: <-op.Ready()
//   - Select with multiple channels: select { case <-op.Ready(): ... case <-ctx.Done(): ... }
//   - Check readiness without blocking: select { case <-op.Ready(): ... default: ... }
//   - Monitor completion: <-op.Completed()
//
// The [Await] function provides syntactic sugar for the common pattern:
//
//	done := causalorder.Await(op)
//	defer done()
//	// ... perform operation ...
//
// # Important Usage Notes
//
// When working with operations, it is crucial to call [Operation.Complete] when
// the operation finishes, regardless of whether it succeeds, fails, or is
// cancelled. Failing to call Complete will cause all dependent operations to
// block indefinitely, potentially causing deadlocks, memory leaks, and goroutine
// leaks.
//
// This package only provides ordering guarantees and does not propagate
// cancellation or error states between operations. If an operation fails, the
// caller should implement their own error handling while still calling
// [Operation.Complete] to maintain the correctness of causal chains.
//
// # Concurrency Notes
//
// The ordering types (TotalOrder, PartialOrder, VectorOrder) are not safe for
// concurrent use. They must be used from a single goroutine that is responsible
// for defining the order of operations. This ensures the ordering semantics are
// well-defined.
//
// However, the [Operation] instances returned by HappensAfter are safe for
// concurrent use. While typically a single goroutine manages an operation's
// lifecycle, the Ready and Completed channels can be safely accessed from
// multiple goroutines when coordination is needed.
//
// For usage examples, see the documentation for each ordering type.
package causalorder