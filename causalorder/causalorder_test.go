package causalorder

import (
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestOrderings(t *testing.T) {
	var tests = []struct {
		HappensAfter func() Operation
	}{}
}

func testOrdering(t *testing.T, ordering TotalOrder) {

}

func TestTotalOrdering_BasicSequentialExecution(t *testing.T) {
	synctest.Run(func() {
		var order TotalOrder
		var executed []int
		var mu sync.Mutex

		// Create 5 operations that should execute in order
		for i := 1; i <= 5; i++ {
			i := i // capture loop variable
			op := order.HappensAfter()

			go func() {
				<-op.Ready()
				mu.Lock()
				executed = append(executed, i)
				mu.Unlock()
				op.Complete()
			}()
		}

		// Wait for all goroutines to complete
		synctest.Wait()

		// Verify execution order
		mu.Lock()
		defer mu.Unlock()
		expected := []int{1, 2, 3, 4, 5}
		if len(executed) != len(expected) {
			t.Fatalf("expected %d operations, got %d", len(expected), len(executed))
		}
		for i, v := range executed {
			if v != expected[i] {
				t.Errorf("operation %d: expected value %d, got %d", i, expected[i], v)
			}
		}
	})
}

func TestTotalOrdering_ConcurrentAccessPattern(t *testing.T) {
	synctest.Run(func() {
		var order TotalOrder
		var executed []int
		var mu sync.Mutex
		var wg sync.WaitGroup

		// Multiple goroutines creating operations concurrently
		numGoroutines := 3
		opsPerGoroutine := 3
		wg.Add(numGoroutines)

		for g := 0; g < numGoroutines; g++ {
			g := g
			go func() {
				defer wg.Done()
				for i := 0; i < opsPerGoroutine; i++ {
					value := g*10 + i
					op := order.HappensAfter()

					go func() {
						<-op.Ready()
						mu.Lock()
						executed = append(executed, value)
						mu.Unlock()
						op.Complete()
					}()
				}
			}()
		}

		wg.Wait()
		synctest.Wait()

		// Verify that all operations executed
		mu.Lock()
		defer mu.Unlock()
		if len(executed) != numGoroutines*opsPerGoroutine {
			t.Fatalf("expected %d operations, got %d", numGoroutines*opsPerGoroutine, len(executed))
		}

		// Verify strict ordering (each value appears only once)
		seen := make(map[int]bool)
		for _, v := range executed {
			if seen[v] {
				t.Errorf("value %d executed multiple times", v)
			}
			seen[v] = true
		}
	})
}

func TestTotalOrdering_OperationLifecycle(t *testing.T) {
	t.Run("Ready channel behavior", func(t *testing.T) {
		synctest.Run(func() {
			var order TotalOrder

			op1 := order.HappensAfter()
			op2 := order.HappensAfter()

			// First operation should be ready immediately
			select {
			case <-op1.Ready():
				// Expected
			default:
				t.Error("first operation should be ready immediately")
			}

			// Second operation should not be ready yet
			select {
			case <-op2.Ready():
				t.Error("second operation should not be ready before first completes")
			default:
				// Expected
			}

			// Complete first operation
			op1.Complete()

			// Now second operation should be ready
			select {
			case <-op2.Ready():
				// Expected
			default:
				t.Error("second operation should be ready after first completes")
			}

			op2.Complete()
		})
	})

	t.Run("Completed channel behavior", func(t *testing.T) {
		synctest.Run(func() {
			var order TotalOrder
			op := order.HappensAfter()

			// Completed channel should not be closed initially
			select {
			case <-op.Completed():
				t.Error("Completed channel should not be closed before Complete() is called")
			default:
				// Expected
			}

			// Call Complete
			op.Complete()

			// Completed channel should now be closed
			select {
			case <-op.Completed():
				// Expected
			default:
				t.Error("Completed channel should be closed after Complete() is called")
			}
		})
	})

	t.Run("Multiple Complete calls are safe", func(t *testing.T) {
		synctest.Run(func() {
			var order TotalOrder
			op := order.HappensAfter()

			// Multiple Complete() calls should not panic
			op.Complete()
			op.Complete()
			op.Complete()

			// Verify channel is still closed
			select {
			case <-op.Completed():
				// Expected
			default:
				t.Error("Completed channel should remain closed")
			}
		})
	})
}

func TestTotalOrdering_ZeroValueInitialization(t *testing.T) {
	synctest.Run(func() {
		var order TotalOrder // Zero value

		// Should work without explicit initialization
		op1 := order.HappensAfter()

		// First operation should proceed immediately
		select {
		case <-op1.Ready():
			// Expected
		default:
			t.Error("first operation on zero-value TotalOrder should be ready immediately")
		}

		op1.Complete()

		// Subsequent operations should also work
		op2 := order.HappensAfter()
		go func() {
			<-op2.Ready()
			op2.Complete()
		}()

		synctest.Wait()
	})
}

func TestTotalOrdering_BlockingAndUnblocking(t *testing.T) {
	t.Run("Operations wait for predecessors", func(t *testing.T) {
		synctest.Run(func() {
			var order TotalOrder
			var executed []int
			var mu sync.Mutex

			ops := make([]Operation, 3)
			for i := range ops {
				ops[i] = order.HappensAfter()
			}

			// Start operations in reverse order
			for i := len(ops) - 1; i >= 0; i-- {
				i := i
				go func() {
					<-ops[i].Ready()
					mu.Lock()
					executed = append(executed, i)
					mu.Unlock()
					time.Sleep(10 * time.Millisecond) // Simulate work
					ops[i].Complete()
				}()
			}

			synctest.Wait()

			// Verify they still executed in correct order
			mu.Lock()
			defer mu.Unlock()
			for i, v := range executed {
				if v != i {
					t.Errorf("position %d: expected %d, got %d", i, i, v)
				}
			}
		})
	})

	t.Run("Chain progression", func(t *testing.T) {
		synctest.Run(func() {
			var order TotalOrder

			op1 := order.HappensAfter()
			op2 := order.HappensAfter()
			op3 := order.HappensAfter()

			var completed atomic.Int32

			go func() {
				<-op1.Ready()
				time.Sleep(50 * time.Millisecond)
				completed.Add(1)
				op1.Complete()
			}()

			go func() {
				<-op2.Ready()
				if completed.Load() != 1 {
					t.Error("op2 started before op1 completed")
				}
				time.Sleep(30 * time.Millisecond)
				completed.Add(1)
				op2.Complete()
			}()

			go func() {
				<-op3.Ready()
				if completed.Load() != 2 {
					t.Error("op3 started before op2 completed")
				}
				completed.Add(1)
				op3.Complete()
			}()

			synctest.Wait()

			if completed.Load() != 3 {
				t.Errorf("expected 3 completed operations, got %d", completed.Load())
			}
		})
	})
}

func TestTotalOrdering_MemoryManagement(t *testing.T) {
	synctest.Run(func() {
		var order TotalOrder
		completedCount := 0
		var mu sync.Mutex

		// Create many sequential operations
		const numOps = 100
		for i := 0; i < numOps; i++ {
			op := order.HappensAfter()
			go func() {
				<-op.Ready()
				// Simulate some work
				time.Sleep(time.Microsecond)
				op.Complete()

				mu.Lock()
				completedCount++
				mu.Unlock()
			}()
		}

		synctest.Wait()

		mu.Lock()
		defer mu.Unlock()
		if completedCount != numOps {
			t.Errorf("expected %d completed operations, got %d", numOps, completedCount)
		}
	})
}

func TestTotalOrdering_EdgeCases(t *testing.T) {
	t.Run("Incomplete operations block chain", func(t *testing.T) {
		synctest.Run(func() {
			var order TotalOrder

			op1 := order.HappensAfter()
			op2 := order.HappensAfter()
			op3 := order.HappensAfter()

			// Start op1 but never complete it
			go func() {
				<-op1.Ready()
				// Never call op1.Complete()
			}()

			// op2 and op3 should remain blocked
			blocked2 := true
			blocked3 := true

			go func() {
				<-op2.Ready()
				blocked2 = false
				op2.Complete()
			}()

			go func() {
				<-op3.Ready()
				blocked3 = false
				op3.Complete()
			}()

			// Give some time for goroutines to run
			time.Sleep(10 * time.Millisecond)

			// Use synctest.Wait to ensure all goroutines are blocked
			synctest.Wait()

			if !blocked2 {
				t.Error("op2 should remain blocked when op1 doesn't complete")
			}
			if !blocked3 {
				t.Error("op3 should remain blocked when op1 doesn't complete")
			}
		})
	})

	t.Run("Rapid fire operations", func(t *testing.T) {
		synctest.Run(func() {
			var order TotalOrder
			var executed []int
			var mu sync.Mutex

			// Create many operations as quickly as possible
			const numOps = 50
			for i := 0; i < numOps; i++ {
				i := i
				op := order.HappensAfter()
				go func() {
					<-op.Ready()
					mu.Lock()
					executed = append(executed, i)
					mu.Unlock()
					op.Complete()
				}()
			}

			synctest.Wait()

			// Verify all executed in order
			mu.Lock()
			defer mu.Unlock()
			if len(executed) != numOps {
				t.Fatalf("expected %d operations, got %d", numOps, len(executed))
			}
			for i, v := range executed {
				if v != i {
					t.Errorf("position %d: expected %d, got %d", i, i, v)
				}
			}
		})
	})

	t.Run("Long chains maintain performance", func(t *testing.T) {
		synctest.Run(func() {
			var order TotalOrder
			const numOps = 200
			completed := 0
			var mu sync.Mutex

			start := time.Now()

			for i := 0; i < numOps; i++ {
				op := order.HappensAfter()
				go func() {
					<-op.Ready()
					op.Complete()
					mu.Lock()
					completed++
					mu.Unlock()
				}()
			}

			synctest.Wait()
			elapsed := time.Since(start)

			mu.Lock()
			defer mu.Unlock()
			if completed != numOps {
				t.Errorf("expected %d completed operations, got %d", numOps, completed)
			}

			// With synctest, this should complete almost instantly
			if elapsed > 100*time.Millisecond {
				t.Logf("Long chain took %v, which might indicate a performance issue", elapsed)
			}
		})
	})
}
