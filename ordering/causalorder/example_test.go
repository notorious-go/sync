package causalorder_test

import (
	"fmt"

	"github.com/notorious-go/sync/ordering"
	"github.com/notorious-go/sync/ordering/causalorder"
)

// This example demonstrates how CausalOrder models causal (i.e. happens-after)
// dependencies between operations as a directed acyclic graph (DAG).
//
// CausalOrder allows operations to depend on multiple "chains" of causality.
// Each chain is identified by a key (in this example, strings like "config" or
// "data"). Operations within the same chain execute sequentially, while
// operations in different chains can execute concurrently.
func ExampleCausalOrder() {
	// When using CausalOrder, users choose the type that identifies the chains
	// of operations. In this example, we will use strings as keys.
	//
	// The zero value is ready to use.
	var graph causalorder.CausalOrder[string]

	// Helper function to check and display operation readiness.
	checkStatus := func(name string, op ordering.Operation) {
		if ordering.Completed(op) {
			fmt.Printf("%s: completed\n", name)
		} else if ordering.Ready(op) {
			fmt.Printf("%s: ready\n", name)
		} else {
			fmt.Printf("%s: blocked\n", name)
		}
	}

	// This case demonstrates an operation that has no dependencies.
	{
		fmt.Println("Case 1: Operation with no dependencies")

		// When HappensAfter is called with no keys, the operation is immediately ready
		// and doesn't participate in any causal chains.
		op1 := graph.HappensAfter() // No keys = no dependencies.
		defer op1.Complete()        // Always remember to Complete() operations!
		checkStatus("op1", op1)

		// Any operations that happen after "no dependencies" are also ready immediately,
		// since there are still no previous operations to wait for.
		op2 := graph.HappensAfter() // Still no dependencies.
		defer op2.Complete()        // Always remember to Complete() operations!
		checkStatus("op2", op2)
	}

	fmt.Println()

	// This case demonstrates how operations with the same key form a chain and must
	// execute in the order they were created.
	{
		fmt.Println("Case 2: Single chain operations")

		// The first operation in any chain is immediately ready because there are no
		// previous operations to wait for.
		first := graph.HappensAfter("work")
		checkStatus("first", first)

		// The second operation with the same key ("work") must wait for the first
		// operation to complete before it can proceed.
		second := graph.HappensAfter("work")
		checkStatus("second", second)

		// Completing the first operation unblocks the second.
		first.Complete()
		checkStatus("second (after first completes)", second)
		second.Complete() // Always remember to Complete() operations!
	}

	fmt.Println()

	// This case demonstrates how a single operation may depend on multiple
	// independent chains, allowing for maximal concurrency, within the constraints
	// of the causal order.
	{
		fmt.Println("Case 3: Multi-chain coordination")

		// Start two independent workflows: loading config and loading data. Since these
		// use different keys, they can proceed concurrently.
		loadConfig := graph.HappensAfter("config")
		loadData := graph.HappensAfter("data")

		// And add subsequent operations to each chain. These must wait for their
		// respective "load" operations to complete.
		//
		// Different chains are useful for modelling independent pipelines that don't
		// interfere with each other.
		useConfig := graph.HappensAfter("config")
		useData := graph.HappensAfter("data")

		fmt.Println("Initial state:")
		checkStatus(" loadConfig", loadConfig) // ready (first in its chain)
		checkStatus(" useConfig", useConfig)   // blocked (waiting for loadConfig)
		checkStatus(" loadData", loadData)     // ready (first in its chain)
		checkStatus(" useData", useData)       // blocked (waiting for loadData)

		// Completing loading configuration will unblock the next operation in the
		// "config" chain, but it will not affect the "data" chain.
		fmt.Println("After loading config:")
		loadConfig.Complete()
		checkStatus(" loadConfig", loadConfig) // now completed
		checkStatus(" useConfig", useConfig)   // now ready
		checkStatus(" loadData", loadData)     // still ready
		checkStatus(" useData", useData)       // still blocked

		// While the chains are independent, a new operation can be added that depends on
		// both chains. This operation will wait for the latest operation in both
		// "config" and "data" chains to be complete before it can proceed.
		//
		// For operations that depend on multiple chains simultaneously, use HappensAfter
		// with multiple keys. Such operations are considered ready if and only if ALL
		// the specified chains are ready.
		combinedOp := graph.HappensAfter("config", "data")
		defer combinedOp.Complete()            // The best practice is to defer Complete().
		checkStatus(" combinedOp", combinedOp) // blocked (waiting for both chains)

		fmt.Println("After loading data and using the config:")
		loadData.Complete()
		useConfig.Complete()
		checkStatus(" useConfig", useConfig)   // now completed
		checkStatus(" loadData", loadData)     // now completed
		checkStatus(" useData", useData)       // now ready
		checkStatus(" combinedOp", combinedOp) // still blocked (waiting for useData)

		fmt.Println("After using data:")
		useData.Complete()
		checkStatus(" useData", useData) // now completed
		// TODO: At this point, combinedOp is not immediately marked as ready because the internal state update happens
		// 	asynchronously. This can and will be fixed in a future release.
		checkStatus(" combinedOp", combinedOp) // now ready
	}

	// Output:
	// Case 1: Operation with no dependencies
	// op1: ready
	// op2: ready
	//
	// Case 2: Single chain operations
	// first: ready
	// second: blocked
	// second (after first completes): ready
	//
	// Case 3: Multi-chain coordination
	// Initial state:
	//  loadConfig: ready
	//  useConfig: blocked
	//  loadData: ready
	//  useData: blocked
	// After loading config:
	//  loadConfig: completed
	//  useConfig: ready
	//  loadData: ready
	//  useData: blocked
	//  combinedOp: blocked
	// After loading data and using the config:
	//  useConfig: completed
	//  loadData: completed
	//  useData: ready
	//  combinedOp: blocked
	// After using data:
	//  useData: completed
	//  combinedOp: ready
}
