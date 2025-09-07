// Package semaphore provides a simple counting semaphore implementation optimized
// for optional concurrency limits, where nil represents unlimited capacity.
//
// # Why This Package Exists
//
// This semaphore type is specifically designed for scenarios where you need optional
// concurrency limiting - where the absence of a limit (nil) means unlimited capacity
// rather than blocking forever. This pattern is particularly useful in libraries like
// errgroup where semaphores may or may not be configured.
//
// Raw buffered channels can serve as semaphores, but they have a critical limitation:
// a nil channel blocks forever on both send and receive operations. This package
// inverts that behavior - a nil Semaphore never blocks, representing unlimited capacity.
// This eliminates the need for defensive `if sem != nil` checks before every acquire
// and release operation, resulting in cleaner, more natural code.
//
// # When NOT to Use This Package
//
// This package implements one very specific semaphore variant. If you need ANY
// functionality beyond what's provided here, you should use alternatives:
//
//   - Weighted semaphores (acquiring multiple tokens at once): Use golang.org/x/sync/semaphore
//   - Context cancellation support: Use raw buffered channels with select statements
//   - Strict FIFO ordering guarantees: Use raw buffered channels without TryAcquire
//   - Waiting for all tokens to be released/acquired: Write your own semaphore variant
//   - Priority queuing, timeouts, or any other features: Implement your own solution
//
// The philosophy here is explicit: there is no one-size-fits-all semaphore implementation.
// Users are encouraged to write their own trivial semaphore flavors tailored to their
// specific needs rather than using an overly generic solution.
//
// # Primary Use Case
//
// This semaphore is designed for limiting the number of concurrently spawned goroutines
// while providing backpressure through both blocking (Acquire) and non-blocking
// (TryAcquire) mechanisms. The specific high-level use depends on what those limited
// goroutines are doing - HTTP request handling, database connections, file operations,
// or any other resource-constrained concurrent work.
//
// # Design Trade-offs
//
// This implementation makes deliberate trade-offs in favor of simplicity and a specific
// use case:
//
//   - No context support: Keeps the API simple and focused
//   - Channel "barging" behavior: TryAcquire may succeed even when others are blocked
//   - No multi-token operations: Each acquire/release handles exactly one token
//   - Exposed channel type: Zero-cost abstraction but requires API discipline
//
// This package was created for internal use in higher-level synchronization mechanisms
// within this project. While it's publicly available, it's deliberately narrow in scope
// and unlikely to fit general-purpose semaphore needs.
//
// # Implementation
//
// The semaphore is implemented as a simple buffered channel where the buffer size
// determines the maximum number of concurrent operations. This provides a zero-cost
// abstraction - the semaphore IS the channel, allowing use of built-in len() and cap()
// functions for inspection.
package semaphore
