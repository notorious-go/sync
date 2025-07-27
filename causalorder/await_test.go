package causalorder_test

import (
	"sync"
	"testing"
	"time"

	"github.com/notorious-go/sync/causalorder"
)

func TestAwait(t *testing.T) {
	t.Run("TotalOrder", func(t *testing.T) {
		var order causalorder.TotalOrder
		var sequence []int
		var mu sync.Mutex

		op1 := order.HappensAfter()
		op2 := order.HappensAfter()
		op3 := order.HappensAfter()

		var wg sync.WaitGroup
		wg.Add(3)

		// Start operations in reverse order to test blocking
		go func() {
			defer wg.Done()
			done := causalorder.Await(op3)
			defer done()
			mu.Lock()
			sequence = append(sequence, 3)
			mu.Unlock()
		}()

		go func() {
			defer wg.Done()
			done := causalorder.Await(op2)
			defer done()
			mu.Lock()
			sequence = append(sequence, 2)
			mu.Unlock()
		}()

		go func() {
			defer wg.Done()
			done := causalorder.Await(op1)
			defer done()
			mu.Lock()
			sequence = append(sequence, 1)
			mu.Unlock()
		}()

		wg.Wait()

		// Check sequence
		if len(sequence) != 3 {
			t.Fatalf("expected 3 operations, got %d", len(sequence))
		}
		for i, val := range sequence {
			if val != i+1 {
				t.Errorf("expected sequence[%d] = %d, got %d", i, i+1, val)
			}
		}
	})

	t.Run("PartialOrder", func(t *testing.T) {
		var order causalorder.PartialOrder[string]
		
		// Create operations for two different keys
		opA1 := order.HappensAfter("A")
		opA2 := order.HappensAfter("A")
		opB1 := order.HappensAfter("B")
		opB2 := order.HappensAfter("B")

		// Track completion order
		completionOrder := make(chan string, 4)

		var wg sync.WaitGroup
		wg.Add(4)

		// Start all operations
		go func() {
			defer wg.Done()
			done := causalorder.Await(opA1)
			defer done()
			time.Sleep(20 * time.Millisecond) // Simulate work
			completionOrder <- "A1"
		}()

		go func() {
			defer wg.Done()
			done := causalorder.Await(opA2)
			defer done()
			completionOrder <- "A2"
		}()

		go func() {
			defer wg.Done()
			done := causalorder.Await(opB1)
			defer done()
			time.Sleep(10 * time.Millisecond) // Simulate work
			completionOrder <- "B1"
		}()

		go func() {
			defer wg.Done()
			done := causalorder.Await(opB2)
			defer done()
			completionOrder <- "B2"
		}()

		wg.Wait()
		close(completionOrder)

		// Collect results
		var results []string
		for result := range completionOrder {
			results = append(results, result)
		}

		// Verify ordering constraints
		// A1 must come before A2
		// B1 must come before B2
		// But A and B operations can interleave
		a1Index, a2Index := -1, -1
		b1Index, b2Index := -1, -1
		
		for i, r := range results {
			switch r {
			case "A1":
				a1Index = i
			case "A2":
				a2Index = i
			case "B1":
				b1Index = i
			case "B2":
				b2Index = i
			}
		}

		if a1Index >= a2Index {
			t.Errorf("A1 should complete before A2, but got indices %d and %d", a1Index, a2Index)
		}
		if b1Index >= b2Index {
			t.Errorf("B1 should complete before B2, but got indices %d and %d", b1Index, b2Index)
		}
	})

	t.Run("MultipleCalls", func(t *testing.T) {
		var order causalorder.TotalOrder
		op := order.HappensAfter()

		done := causalorder.Await(op)
		
		// Calling done multiple times should be safe
		done()
		done()
		done()

		// Operation should be completed
		select {
		case <-op.Completed():
			// Expected
		default:
			t.Error("operation should be completed")
		}
	})
}