// Package causalorder provides synchronization primitives that maintain causal
// consistency when processing events across multiple concurrent streams.
//
// This package addresses the challenge of processing a partially ordered set of
// events where some events must be processed after certain other events, while
// maximizing concurrent processing of independent events. Many concurrent
// systems need to respect causal dependencies while processing events in
// parallel to achieve high throughput. The standard sync.WaitGroup can
// coordinate concurrent operations but provides no ordering guarantees.
// Single-key partitioning schemes like those in Kafka can maintain order within
// a partition but limit each event to exactly one stream. This package enables
// events to participate in multiple independent processing streams while
// ensuring all causal dependencies are respected.
//
// # Prerequisites
//
// The package assumes:
//   - A monotonic source of events observed sequentially
//   - Each event may have happens-after relationships with previously observed
//     events
//   - Events can be assigned to one or more streams (queues or topics)
//   - Within each stream, events maintain strict ordering
//
// # Core Concepts
//
// The package uses a hierarchy of abstractions to manage event ordering:
//
// A stream is either a queue (unpartitioned, maintaining total order) or a
// topic (partitioned into multiple chains, exhibiting partial order). When
// events are assigned to queues, they form a single sequence. When assigned to
// topics, they are distributed across partitions based on a partition key, with
// each partition maintaining its own total order.
//
// Internally, all streams resolve to chains - the fundamental unit of total
// ordering. A queue maps to a single chain, while a topic maps to multiple
// chains (one per partition). The Sequence type manages these chains and their
// interdependencies.
//
// An event becomes eligible for processing only when it reaches the head
// position in all chains it belongs to. This position reflects both explicit
// happens-after dependencies and implicit ordering within each chain.
//
// # Types
//
// Sequence provides channel-based synchronization for streaming event
// processing. Events are submitted to the sequence along with their stream
// assignments (queues and/or topics with partition keys). The sequence returns
// a wait channel and done function, allowing fine-grained control over event
// processing while maintaining all ordering guarantees.
//
// Group provides synchronous execution similar to sync.WaitGroup but with
// causal ordering guarantees. Functions submitted to a Group execute once their
// dependencies complete, allowing structured concurrent execution that respects
// happens-after relationships.
//
// Both types ensure that the transitive closure of the happens-after relation
// is respected: if event A happens-after B, and B happens-after C, then A will
// be processed after both B and C, even if the A-C relationship is not
// explicitly declared.
package causalorder