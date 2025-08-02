// Package causalgroup provides concurrent execution primitives that maintain
// causal ordering constraints while allowing parallelism where possible.
//
// The package offers three main types for different ordering requirements:
//
// [Queue] provides strict sequential execution where each operation waits for
// all previous operations to complete. This is useful for tasks that must be
// processed in exact order, such as database migrations or transaction logs.
//
// [Topic] provides partitioned execution where operations are grouped by a key.
// Operations with the same key execute sequentially, but operations with
// different keys can run concurrently. This is ideal for processing events or
// commands that are independent across different entities, such as user actions
// or message queues partitioned by topic.
//
// [Matrix] provides complex dependency-based execution where operations can
// depend on multiple other operations. Each operation blocks until all
// operations it depends on have completed. This enables building directed
// acyclic graphs of dependencies, similar to build systems like Make.
//
// All types support limiting the number of concurrent goroutines through
// SetLimit, allowing control over resource usage while maintaining the specified
// ordering constraints.
//
// # Basic Usage
//
// Each type follows a similar pattern:
//
//  1. Create an instance (zero value is valid)
//  2. Optionally set a concurrency limit
//  3. Submit work items sequentially using the Go() method
//  4. Wait for all work to complete
//
// Work items are submitted from a single, ordered source. The ordering comes
// from the sequential nature of the submission, not from concurrent calls.
//
// # Sequential Submission Requirement
//
// IMPORTANT: The Go method on all types must be called sequentially from a
// single goroutine or ordered source. Calling Go() concurrently from multiple
// goroutines defeats the purpose of these types, as the concurrent submission is
// inherently unordered.
//
// These types are NOT designed for concurrent submission. They are designed
// to manage concurrent execution of work items that arrive in a strict order.
// The causal relationships between work items are determined by their submission
// order - work item B depends on work item A if B was submitted after A (and
// they share the same partition key for Topic, or share common keys for Matrix).
//
// # Concurrency Limits
//
// By default, there is no limit on concurrent goroutines. Use SetLimit to
// control the maximum number of active goroutines:
//
//	var q causalgroup.Queue
//	q.SetLimit(10) // Allow at most 10 concurrent operations.
//
// Setting a limit of 1 effectively serializes all operations, while negative
// values indicate no limit. A limit of 0 blocks all operations.
//
// # Safety and Best Practices
//
// While the submitted work items execute concurrently (respecting their
// dependencies), the submission itself must be sequential. This is typically
// achieved by having a single consumer reading from an ordered source.
//
// The SetLimit method must not be called while operations are active. Always
// set limits before submitting work or after calling Wait.
//
// When choosing between types:
//   - Use [Queue] when order matters globally.
//   - Use [Topic] when order matters within partitions.
//   - Use [Matrix] when you have complex interdependencies.
package causalgroup
