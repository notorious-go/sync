package partialorder_test

import (
	"fmt"
	"sync"

	"github.com/notorious-go/sync/ordering/partialorder"
)

// This example demonstrates how to effectively process a stream of events with
// partial causal ordering by showing a sample server that audits actions in the
// correct order for the same user, even when processed concurrently.
//
// The [PartialOrder] and [Topic] types support any number of partitions,
// allowing you to manage dynamic sets of dependent operations.
//
// When orchestration requires fine-grained control over the lifecycle of
// operations, you can use the [PartialOrder] type directly. The [Topic] type is
// a higher-level construct that demonstrates one way to expose a simpler API to
// manage a group of goroutines with partitioned dependencies.
func Example() {
	// For this example, we will use a partial group (called a Topic) to ensure that
	// actions for the same user are logged in order, even when started concurrently.
	//
	// Note that any comparable type can be used as a key for the PartialOrder.
	//
	// Note how the zero value of PartialOrder is ready to use.
	var topic partialorder.Topic[string]

	// This example uses a simple in-memory audit log to demonstrate the causal
	// ordering of user actions.
	var database AuditLog

	for _, event := range EventSource {
		// With a Topic, the Go method allows us to dynamically assign operations to
		// specific keys (in this case, usernames) while managing concurrency and
		// happens-after ordering.
		//
		// Without limits, Go spawns a new goroutine for each event and returns
		// immediately. In a real application, you may want to set a limit on the number
		// of active goroutines to avoid overwhelming the system, though it will not
		// affect the correctness of the ordering.
		topic.Go(event.User, func() {
			// The given function is called only after all previous operations for the same
			// user have completed. This is achieved by the Topic internally managing the
			// PartialOrder and synchronizing the operations.
			//
			// After the internal Ready channel synchronizes, we know that no other goroutine
			// is currently processing actions for the same user, so we can safely apply the
			// change to the database without additional locking for the same user.
			database.Append(event.User, event.Action)
		})
	}

	// Just like with WaitGroup, we wait for all operations to complete before
	// printing the database state.
	topic.Wait()
	fmt.Println(database.String())

	// Unordered Output:
	// User alice: [login binge logout]
	// User bob: [login logout]
}

// EventSource is a sample log of user actions that we will use to demonstrate
// causal ordering.
//
// This data is synthetic, but in a real application, it would come from an event
// source like a message queue or a database change stream.
//
// Each action is associated with a user, and we want to ensure that actions for
// the same user are audited in the order they were performed, even if the events
// are processed concurrently.
var EventSource = []struct {
	User   string
	Action string
}{
	{User: "alice", Action: "login"},
	{User: "bob", Action: "login"},
	{User: "alice", Action: "binge"},
	{User: "bob", Action: "logout"},
	{User: "alice", Action: "logout"},
}

// AuditLog is a simple in-memory log that records user actions in the order they
// were appended. The Append method is thread-safe and allows concurrent
// appending of actions by different users, and the String method returns a
// formatted string representation of the log.
type AuditLog struct {
	m  map[string][]string
	mu sync.Mutex
}

func (l *AuditLog) Append(user, action string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m == nil {
		l.m = make(map[string][]string)
	}
	l.m[user] = append(l.m[user], action)
}

func (l *AuditLog) String() string {
	var s string
	for user, actions := range l.m {
		s += fmt.Sprintf("User %v: %v\n", user, actions)
	}
	return s
}
