package causalgroup_test

import "fmt"

// RecordEvent represents an event in a stream of events related to records.
type RecordEvent struct {
	Id int    // The identifier of the record that changed.
	Op string // The operation that was performed on the record.
}

// ExampleQueue demonstrates how to use the Queue type
// to process multiple event chains from a stream, wherein each chain follows a strict total order of causality (i.e. "happends-after" relation).
func ExampleQueue() {
	// A zero Queue is ready to use.
	// The generic type parameter allows for any comparable type to be used as identifier for events in the chain.
	var q causalorder.QueueGroup[int]
	// Call SetLimit to set the maximum number of concurrent goroutines.
	// This is optional; if not set, the default is no limit.
	// Here we set it to 1 to ensure stable output for the example.
	// In a real application, there's of course no value in limiting the number
	// of concurrent goroutines to 1, which would defeat the purpose of using this type.
	q.SetLimit(1)

	// For this example, we simulate a stream of events about records. Each key is a record identifier,
	// and the value is the event that happened to that record.
	eventStream := []RecordEvent{
		{Id: 1, Op: "created"},
		{Id: 2, Op: "created"},
		{Id: 1, Op: "updated"},
		{Id: 2, Op: "deleted"},
	}

	// Call Go synchronously to preserve the order of events, as received by a source, while "allowing" concurrent processing.
	// Though a Queue allows for concurrent processing, it actually prevents each goroutine
	// from proceeding until the previous event has been processed, thus maintaining the total order.
	for _, event := range eventStream {
		q.Go(event, func() {
			// -- do something with the event --
			// For demonstration, we just print the event.
			// In a real application, this could be writing to a database, sending a message, acknowledging an event, etc.
			fmt.Printf("Processing event %d: %s\n", event.Id, event.Op)
		})
	}

	// Output:
	// Processing event 1: created
	// Processing event 2: created
	// Processing event 1: updated
	// Processing event 2: deleted
}
