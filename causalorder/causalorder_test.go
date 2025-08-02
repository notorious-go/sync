package causalorder

import (
	"fmt"
	"slices"
	"sync"
	"testing"
)

// TestTotalOrdering tests that all operations are executed in a total order,
//
// This test processes the following events in the following order:
//
//  1. 1️⃣, which happens immediately.
//  2. 2️⃣, which happens after 1️⃣.
//  3. 3️⃣, which happens after 2️⃣, and transitively after 1️⃣.
//  4. 4️⃣, which happens after 3️⃣, and transitively after 2️⃣ and 1️⃣.
func TestTotalOrdering(t *testing.T) {
	var ordering TotalOrder
	var events = []TestEvent{
		{
			Token:        "1️⃣",
			Dependencies: nil,
			Operation:    ordering.HappensAfter(),
		},
		{
			Token:        "2️⃣",
			Dependencies: []string{"1️⃣"},
			Operation:    ordering.HappensAfter(),
		},
		{
			Token:        "3️⃣",
			Dependencies: []string{"2️⃣", "1️⃣"},
			Operation:    ordering.HappensAfter(),
		},
		{
			Token:        "4️⃣",
			Dependencies: []string{"3️⃣", "2️⃣", "1️⃣"},
			Operation:    ordering.HappensAfter(),
		},
	}
	runTestCase(t, events)
}

// TestPartialOrdering tests that operations for the same key are executed in the
// correct order, while operations for different keys can run concurrently
// without affecting each other.
//
// This test processes the following events in the following order:
//
//  1. A1, which happens immediately.
//  2. B1, which happens immediately.
//  3. A2, which happens after A1.
//  4. B2, which happens after B1.
//  5. A3, which happens after A2 (and transitively after A1).
func TestPartialOrdering(t *testing.T) {
	var ordering PartialOrder[string]
	events := []TestEvent{
		{
			Token:        "A1",
			Dependencies: nil,
			Operation:    ordering.HappensAfter("A"),
		},
		{
			Token:        "B1",
			Dependencies: nil,
			Operation:    ordering.HappensAfter("B"),
		},
		{
			Token:        "A2",
			Dependencies: []string{"A1"},
			Operation:    ordering.HappensAfter("A"),
		},
		{
			Token:        "B2",
			Dependencies: []string{"B1"},
			Operation:    ordering.HappensAfter("B"),
		},
		{
			Token:        "A3",
			Dependencies: []string{"A2", "A1"},
			Operation:    ordering.HappensAfter("A"),
		},
	}
	runTestCase(t, events)
}

// This test case processes the following events in the following order:
//
//  1. (A∩1), which happens immediately.
//  2. (B∩2), which happens immediately.
//  3. (A∩2), which happens after (A∩1) and (B∩2).
//  4. (B∩1), which happens after (A∩1) and (B∩2).
//  5. (B∩2)′, which happens after (A∩2) and (B∩1), and transitively after (A∩1) and (B∩2).
func TestVectorOrdering(t *testing.T) {
	var ordering VectorOrder[string]
	events := []TestEvent{
		{
			Token:        "(A∩1)",
			Dependencies: nil,
			Operation:    ordering.HappensAfter("A", "1"),
		},
		{
			Token:        "(B∩2)",
			Dependencies: nil,
			Operation:    ordering.HappensAfter("B", "2"),
		},
		{
			Token:        "(A∩2)",
			Dependencies: []string{"(A∩1)", "(B∩2)"},
			Operation:    ordering.HappensAfter("A", "2"),
		},
		{
			Token:        "(B∩1)",
			Dependencies: []string{"(A∩1)", "(B∩2)"},
			Operation:    ordering.HappensAfter("B", "1"),
		},
		{
			Token: "(B∩2)′",
			Dependencies: []string{
				"(A∩2)", "(B∩1)", // Direct dependencies.
				"(A∩1)", "(B∩2)", // Transitive dependencies.
			},
			Operation: ordering.HappensAfter("B", "2"),
		},
	}
	runTestCase(t, events)
}

// runTestCase executes an event-loop consisting of the predefined events
// concurrently and verifies that all dependency constraints are satisfied.
//
// The function:
//
//   - Spawns a goroutine for each event in reverse order (to stress the ordering).
//   - Each goroutine waits for its Operation to be ready before proceeding.
//   - Records the actual execution order of events, which is the order in which
//     the spawned goroutines are running.
//   - Verifies that the declared order was satisfied using TestEvent.Check.
//
// This helper is designed to test the correctness of TotalOrder, PartialOrder,
// and VectorOrder implementations by ensuring that operations respect their
// declared dependencies when executing concurrently.
func runTestCase(t *testing.T, events []TestEvent) {
	t.Helper()

	var (
		mu     sync.Mutex
		tokens []string
	)

	var wg sync.WaitGroup
	for _, event := range slices.Backward(events) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer event.Operation.Complete()

			select {
			case <-event.Operation.Ready():
				// The operation is ready, meaning all dependencies are satisfied.
				t.Logf("Processing event %s", event.Token)
			case <-t.Context().Done():
				t.Errorf("test interrupted before event %s could be processed", event.Token)
			}

			// Collect the token for verification.
			mu.Lock()
			tokens = append(tokens, event.Token)
			mu.Unlock()
		}()
	}
	wg.Wait()

	// Verify that all tokens were processed in the expected order.
	for _, event := range events {
		event.Check(t, tokens)
	}
}

// TestEvent represents a step in a concurrent [TestCase] that uses either
// TotalOrder, PartialOrder, or VectorOrder to test the causal relationships
// between operations.
//
// Each event has a token that identifies it, a list of dependencies that
// represent other events that must complete before this event can be processed,
// and an Operation that defines the "happens-after" relationship for this event.
//
// The ordering types are designed to compose dependencies before the processing
// begins in separate goroutines, which is why definition functions (returning
// the events of test-cases) can prepare the "happens-after" relationships
// without actually executing the operations.
//
// The TestCase runner executes the operations themselves.
type TestEvent struct {
	Token        string
	Dependencies []string
	Operation    Operation
}

// Check verifies that all of this event's dependencies were processed before
// this event in the given execution order.
//
// The tokens parameter should contain the ordered list of event tokens as they
// were actually processed. This method will verify that:
//
//   - This event's token appears in the recorded list of tokens
//   - All dependencies listed in .Dependencies appear before .Token in the list
//
// Any violations of the dependency constraints will be reported as test errors.
func (e TestEvent) Check(t *testing.T, tokens []string) {
	t.Helper()

	// Find the position of this event in the list of tokens.
	eventIndex, ok := e.index(tokens)
	if !ok {
		t.Errorf("event %v was not processed", e.Token)
		return
	}

	// Check that all dependencies appear before this event.
	for _, dep := range e.Dependencies {
		found := false
		for i := 0; i < eventIndex; i++ {
			if tokens[i] == dep {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("event %v: dependency %v was not processed before it", e.Token, dep)
		}
	}
}

// Finds the index of this event's token in the given slice of tokens.
func (e TestEvent) index(tokens []string) (index int, found bool) {
	for i, token := range tokens {
		if token == e.Token {
			return i, true
		}
	}
	return 0, false
}

// TestVectorOrderingUnderStress tests VectorOrder with ~100 events representing
// file operations in a version control system.
//
// The test generates:
//   - 5 directory creation events (no dependencies)
//   - 100 file version events (5 dirs × 4 files × 5 versions)
//
// Each file version depends on:
//   - Its parent directory existing (e.g. "src/main-v1" needs "mkdir-src")
//   - Its previous version if any (e.g. "src/main-v2" needs "src/main-v1")
//
// This creates a realistic VectorOrder scenario where operations share some
// dependencies (directory) but maintain independent version chains per file.
func TestVectorOrderingUnderStress(t *testing.T) {
	type fsChain struct {
		Kind string // Either "dir" or "file".
		Name string // Name of the directory or file.
	}
	var ordering VectorOrder[fsChain]

	// Generate events: each file has versions that depend on the directory being
	// created and the previous version of the file.
	var (
		events []TestEvent

		dirs     = []string{"src", "test", "docs", "lib", "bin"}
		files    = []string{"main", "util", "config", "data"}
		versions = 5
	)

	// Create directories first (no dependencies).
	for _, dir := range dirs {
		dirChain := fsChain{Kind: "dir", Name: dir}
		dirToken := fmt.Sprintf("mkdir-%s", dir)
		events = append(events, TestEvent{
			Token:     dirToken,
			Operation: ordering.HappensAfter(dirChain),
		})

		// Create file versions: each depends on its directory and previous version.
		for _, file := range files {
			fileChain := fsChain{Kind: "file", Name: file}
			for v := 1; v <= versions; v++ {
				token := fmt.Sprintf("%s/%s-v%d", dir, file, v)
				deps := []string{dirToken}
				if v > 1 {
					deps = append(deps, fmt.Sprintf("%s/%s-v%d", dir, file, v-1))
				}
				events = append(events, TestEvent{
					Token:        token,
					Dependencies: deps,
					Operation:    ordering.HappensAfter(dirChain, fileChain),
				})
			}
		}
	}

	t.Logf("Testing VectorOrder with %d events", len(events))
	runTestCase(t, events)
}
