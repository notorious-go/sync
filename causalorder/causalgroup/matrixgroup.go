package causalgroup

// TryGo calls the given function in a new goroutine only if the number of
// active goroutines in the group is currently below the configured limit.
//
// Like Go, the new goroutine will block before calling f until all previously
// submitted tasks that share any of the given keys have completed.
//
// The return value reports whether the goroutine was started. If it returns
// false, then the underlying causal operation was never started.
func (m *Matrix[K]) TryGo(keys []K, f func()) (started bool) {
	// Note: this allows barging iff channels in general allow barging; see
	// the comment on TryAcquire for a bit more details.
	if !m.sem.TryAcquire() {
		return false
	}
	m.wg.Add(1)
	op := m.ordering.HappensAfter(keys...)
	go func() {
		defer m.done(op)
		<-op.Ready()
		f()
	}()
	return true
}
