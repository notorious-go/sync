package ordering

// Operation represents a unit of work in a causally ordered execution chain. It
// provides synchronization points for managing when an operation can begin and
// signalling when it has completed.
//
// An Operation is the fundamental building block for enforcing "happens-after"
// relationships in concurrent programs. Each Operation waits for its causal
// dependencies to complete before becoming ready and signals its own completion
// to unblock dependent operations.
//
// The Operation interface provides a channel-based API that enables flexible
// synchronization patterns:
//   - Block until ready using <-op.Ready()
//   - Select with multiple channels: select { case <-op.Ready(): ... case <-ctx.Done(): ... }
//   - Check readiness without blocking: select { case <-op.Ready(): ... default: ... }
//   - Monitor completion: <-op.Completed()
//
// Operations are safe for concurrent use. While typically a single goroutine
// manages an operation's lifecycle, the channels can be safely accessed from
// multiple goroutines when coordination is needed.
type Operation interface {
	// Ready returns a channel that closes when all causal dependencies have
	// completed, signalling that this operation may begin execution.
	//
	// The channel is closed exactly once and remains closed thereafter. Multiple
	// goroutines may safely wait on this channel.
	//
	// For the first operation in a chain, this channel is closed immediately,
	// allowing it to proceed without waiting.
	Ready() <-chan struct{}

	// Completed returns a channel that closes when this operation has been marked
	// as complete via the Complete method.
	//
	// The channel is closed when Complete is called for the first time and
	// remains closed thereafter.
	Completed() <-chan struct{}

	// Complete marks this operation as finished, closing the Completed channel and
	// allowing any causally dependent operations to proceed.
	//
	// This method MUST be called at least once when the operation finishes, whether
	// it succeeds, fails, or is cancelled. Failing to call Complete will cause all
	// dependent operations to block indefinitely, potentially causing goroutine
	// and memory leaks.
	//
	// Users are encouraged to use defer op.Complete() immediately after starting an
	// operation to ensure completion even in case of errors or early returns.
	//
	// Complete is safe to call multiple times - subsequent calls are no-ops.
	// However, for clarity and correctness, it should typically be called exactly
	// once per operation.
	Complete()
}
