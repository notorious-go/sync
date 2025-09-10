package totalorder_test

import (
	"fmt"

	"github.com/notorious-go/sync/ordering/totalorder"
)

// This example demonstrates using Queue to process events in strict sequential
// order, ensuring that each event is fully processed before the next one begins.
func ExampleQueue() {
	// The zero value of Queue is valid and has no limit on the number of active
	// goroutines.
	var queue totalorder.Queue

	// Simulate processing a series of database migrations that must run in order.
	migrations := []string{"create_users", "add_email_index", "create_posts", "add_foreign_keys"}
	for i, migration := range migrations {
		queue.Go(func() {
			// Simulate migration work
			fmt.Printf("Running migration %d: %s\n", i+1, migration)
		})
	}

	// After submitting all migrations, we wait for all goroutines in the queue to
	// complete.
	queue.Wait()
	fmt.Println("All migrations completed")

	// Output:
	// Running migration 1: create_users
	// Running migration 2: add_email_index
	// Running migration 3: create_posts
	// Running migration 4: add_foreign_keys
	// All migrations completed
}
