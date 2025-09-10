package partialorder_test

import (
	"fmt"

	"github.com/notorious-go/sync/ordering/partialorder"
)

// This example demonstrates using Topic to process events partitioned by a key,
// where events with the same key execute sequentially, but events with different
// keys can execute concurrently.
func ExampleTopic() {
	// The zero value of Topic is valid and has no limit on the number of active
	// goroutines.
	var topic partialorder.Topic[string]

	// Simulate processing user actions where each user's actions must be processed
	// in order, but different users can have their actions processed concurrently.
	events := []struct {
		User   string
		Action string
	}{
		{"alice", "login"},
		{"bob", "login"},
		{"alice", "watch-some-reels"},
		{"bob", "binge-some-reels"},
		{"bob", "logout"},
		{"alice", "logout"},
	}

	// Since users are processed concurrently with each other, we cannot guarantee a
	// constant output for this example, so we record the order of actions for each user
	// individually to verify that each user's actions are processed sequentially.
	//
	// We use two slices without locks because the Topic guarantees that actions for
	// the same user will not interleave, so we can safely append to the slices
	// without additional synchronization.
	var alice, bob []string
	for _, event := range events {
		topic.Go(event.User, func() {
			switch event.User {
			case "alice":
				alice = append(alice, event.Action)
			case "bob":
				bob = append(bob, event.Action)
			default:
				panic(fmt.Sprintf("unexpected user: %s", event.User))
			}
		})
	}

	// After submitting all events, we wait for all goroutines in the topic to
	// complete.
	topic.Wait()
	fmt.Println("All user events processed")

	// Verify that actions for each user were processed in order by
	// printing the collected actions for each user.
	fmt.Printf("Alice's actions: %v\n", alice)
	fmt.Printf("Bob's actions: %v\n", bob)

	// Output:
	// All user events processed
	// Alice's actions: [login watch-some-reels logout]
	// Bob's actions: [login binge-some-reels logout]
}
