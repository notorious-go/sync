package causalorder_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/notorious-go/sync/causalorder"
)

// ExampleTotalOrder shows how to ensure database writes happen in order,
// even when started concurrently. The zero value is ready to use.
func ExampleTotalOrder() {
	var order causalorder.TotalOrder
	var wg sync.WaitGroup

	// Start three writes concurrently
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			h := order.Put()
			<-h.Wait() // Wait for previous operations to complete
			fmt.Printf("Writing record %d\n", id)
			time.Sleep(10 * time.Millisecond) // Simulate work
			h.Done()
		}(i)
	}

	wg.Wait()
	// Output:
	// Writing record 1
	// Writing record 2
	// Writing record 3
}

// ExamplePartialOrder shows how to maintain order within each partition
// while allowing different partitions to proceed independently.
// The zero value is ready to use.
func ExamplePartialOrder() {
	var order causalorder.PartialOrder

	// Start all operations
	hA1 := order.Put("alice")
	hA2 := order.Put("alice")
	hA3 := order.Put("alice")
	hB1 := order.Put("bob")
	hB2 := order.Put("bob")

	// Check which operations are ready
	select {
	case <-hA1.Wait():
		fmt.Println("alice: login")
		hA1.Done()
	default:
		fmt.Println("alice login blocked")
	}

	select {
	case <-hB1.Wait():
		fmt.Println("bob: login")
		hB1.Done()
	default:
		fmt.Println("bob login blocked")
	}

	select {
	case <-hA2.Wait():
		fmt.Println("alice: purchase")
		hA2.Done()
	default:
		fmt.Println("alice purchase blocked")
	}

	select {
	case <-hB2.Wait():
		fmt.Println("bob: logout")
		hB2.Done()
	default:
		fmt.Println("bob logout blocked")
	}

	select {
	case <-hA3.Wait():
		fmt.Println("alice: logout")
		hA3.Done()
	default:
		fmt.Println("alice logout blocked")
	}

	// Output:
	// alice: login
	// bob: login
	// alice: purchase
	// bob: logout
	// alice: logout
}
