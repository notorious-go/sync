package causalorder

import (
	"sync"
)

type chain struct {
	head chan struct{}
}

func (c *chain) Put() (wait <-chan struct{}, done chan<- struct{}) {
	// The first time we call Put, the wait channel is immediately ready.
	if c.head == nil {
		c.head = make(chan struct{})
		close(c.head) // Close the channel immediately to signal readiness.
	}

	// The wait channel is the head of the chain set by the last Put call.
	// The done channel is the new head of the chain, which will be used by
	// callers to signal completion for the next Put call.
	wait = c.head
	c.head = make(chan struct{})
	done = c.head
	return wait, done
}

type Handle struct {
	wait     <-chan struct{}
	done     chan<- struct{}
	whenDone func()
}

func (h *Handle) Wait() <-chan struct{} {
	return h.wait
}

func (h *Handle) Done() {
	close(h.done)
	if h.whenDone != nil {
		h.whenDone()
	}
}

func (h *Handle) Ready() bool {
	select {
	case <-h.wait:
		return true
	default:
		return false
	}
}

type TotalOrder struct {
	mu    sync.Mutex
	chain chain
}

func (o *TotalOrder) Put() Handle {
	o.mu.Lock()
	defer o.mu.Unlock()
	wait, done := o.chain.Put()
	return Handle{wait: wait, done: done}
}

type PartialOrder struct {
	mu     sync.Mutex
	chains map[string]*chain
}

func (o *PartialOrder) Put(key string) Handle {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, exists := o.chains[key]; !exists {
		o.chains[key] = &chain{}
	}
	wait, done := o.chains[key].Put()
	return Handle{
		wait: wait,
		done: done,
		whenDone: func() {
			o.mu.Lock()
			defer o.mu.Unlock()
			chain, ok := o.chains[key]
			if !ok {
				// Chain already deleted. This can happen if Handle.Done() is called
				// multiple times, or out of order. This is not the common case.
				return
			}
			if chain.head != done {
				// If the head is not the done channel that was just closed,
				// it means another Put is in progress, so we should not delete this chain.
				return
			}
			delete(o.chains, key)
		},
	}
}

func WaitAll(handles ...Handle) (c <-chan struct{}, cancel func()) {
	quit := make(chan struct{}, 1)
	ready := make(chan struct{})
	go func() {
		for _, h := range handles {
			select {
			case <-quit:
				return
			case <-h.Wait():
			}
		}
		close(ready)
	}()
	return ready, func() {
		close(quit)
	}
}
