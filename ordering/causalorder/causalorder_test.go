package causalorder_test

import (
	"fmt"
	"testing"

	"github.com/notorious-go/sync/ordering/causalorder"
	"github.com/notorious-go/sync/ordering/ordertest"
)

// This test verifies complex chains of operations (centered around "alice" and
// "bob") to ensure that operations within the same chain correctly adhere to the
// interface guarantees.
func TestOperationInterface(t *testing.T) {
	var order causalorder.CausalOrder[string]
	t.Run("alice-in-chains", func(t *testing.T) {
		first := order.HappensAfter("alice", "in", "chains")
		second := order.HappensAfter("alice", "in", "chains")
		ordertest.TestOperationInterface(t, first, second)
	})
	t.Run("bob-in-chains", func(t *testing.T) {
		first := order.HappensAfter("bob", "in", "chains")
		second := order.HappensAfter("bob", "in", "chains")
		ordertest.TestOperationInterface(t, first, second)
	})
}

// TestVectorOrdering tests CausalOrder with multiple overlapping dependencies.
//
// This test case processes the following events in the following order:
//  1. (A∩1), which happens immediately.
//  2. (B∩2), which happens immediately.
//  3. (A∩2), which happens after (A∩1) and (B∩2).
//  4. (B∩1), which happens after (A∩1) and (B∩2).
//  5. (B∩2)′, which happens after (A∩2) and (B∩1), and transitively after (A∩1) and (B∩2).
func TestVectorOrdering(t *testing.T) {
	var ordering causalorder.CausalOrder[string]
	events := []ordertest.Event{
		{
			Token:        "(A∩1)",
			HappensAfter: nil,
			Operation:    ordering.HappensAfter("A", "1"),
		},
		{
			Token:        "(B∩2)",
			HappensAfter: nil,
			Operation:    ordering.HappensAfter("B", "2"),
		},
		{
			Token:        "(A∩2)",
			HappensAfter: []string{"(A∩1)", "(B∩2)"},
			Operation:    ordering.HappensAfter("A", "2"),
		},
		{
			Token:        "(B∩1)",
			HappensAfter: []string{"(A∩1)", "(B∩2)"},
			Operation:    ordering.HappensAfter("B", "1"),
		},
		{
			Token: "(B∩2)′",
			HappensAfter: []string{
				"(A∩2)", "(B∩1)", // Direct dependencies.
				"(A∩1)", "(B∩2)", // Transitive dependencies.
			},
			Operation: ordering.HappensAfter("B", "2"),
		},
	}
	ordertest.Test(t, events)
}

// TestCausalOrderUnderStress tests CausalOrder with ~100 events
// representing file operations in a version control system.
//
// The test generates:
//   - 5 directory creation events (no dependencies)
//   - 100 file version events (5 dirs × 4 files × 5 versions)
//
// Each file version depends on:
//   - Its parent directory existing (e.g. "src/main-v1" needs "mkdir-src")
//   - Its previous version if any (e.g. "src/main-v2" needs "src/main-v1")
//
// This creates a realistic CausalOrder scenario where operations share some
// dependencies (directory) but maintain independent version chains per file.
func TestCausalOrderUnderStress(t *testing.T) {
	type fsChain struct {
		Kind string // Either "dir" or "file".
		Name string // Name of the directory or file.
	}
	var ordering causalorder.CausalOrder[fsChain]

	// Generate events: each file has versions that depend on the directory being
	// created and the previous version of the file.
	var (
		events []ordertest.Event

		dirs     = []string{"src", "test", "docs", "lib", "bin"}
		files    = []string{"main", "util", "config", "data"}
		versions = 5
	)

	// Create directories first (no dependencies).
	for _, dir := range dirs {
		dirChain := fsChain{Kind: "dir", Name: dir}
		dirToken := fmt.Sprintf("mkdir-%s", dir)
		events = append(events, ordertest.Event{
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
				events = append(events, ordertest.Event{
					Token:        token,
					HappensAfter: deps,
					Operation:    ordering.HappensAfter(dirChain, fileChain),
				})
			}
		}
	}

	t.Logf("Testing CausalOrder with %d events", len(events))
	ordertest.Test(t, events)
}
