package causalorder_test

import (
	"fmt"
	"testing"

	"github.com/notorious-go/sync/causalorder"
	"github.com/notorious-go/sync/causalorder/ordertest"
)

// This test case processes the following events in the following order:
//
//  1. (A∩1), which happens immediately.
//  2. (B∩2), which happens immediately.
//  3. (A∩2), which happens after (A∩1) and (B∩2).
//  4. (B∩1), which happens after (A∩1) and (B∩2).
//  5. (B∩2)′, which happens after (A∩2) and (B∩1), and transitively after (A∩1) and (B∩2).
func TestVectorOrdering(t *testing.T) {
	var ordering causalorder.VectorOrder[string]
	events := []ordertest.Event{
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
	ordertest.Test(t, events)
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
	var ordering causalorder.VectorOrder[fsChain]

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
					Dependencies: deps,
					Operation:    ordering.HappensAfter(dirChain, fileChain),
				})
			}
		}
	}

	t.Logf("Testing VectorOrder with %d events", len(events))
	ordertest.Test(t, events)
}
