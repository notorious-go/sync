package causalgroup

// TryGo calls the given function in a new goroutine only if the number of
// active goroutines in the group is currently below the configured limit.
//
// Like Go, the new goroutine will block before calling f until all previously
// submitted tasks with the same partition key have completed.
//
// The return value reports whether the goroutine was started. If it returns
// false, then the underlying causal operation was never started.
func (t *Topic[K]) TryGo(partition K, f func()) (started bool) {
	// Note: this allows barging iff channels in general allow barging; see
	// the comment on TryAcquire for a bit more details.
	if !t.sem.TryAcquire() {
		return false
	}
	t.wg.Add(1)
	op := t.ordering.HappensAfter(partition)
	go func() {
		defer t.done(op)
		<-op.Ready()
		f()
	}()
	return true
}
