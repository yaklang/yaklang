package aicommon

import (
	"context"
	"sync"
	"time"
)

type EndpointSignal struct {
	ch   chan struct{}
	once sync.Once
}

func NewEndpointSignal() *EndpointSignal {
	return &EndpointSignal{
		ch: make(chan struct{}),
	}
}

func (s *EndpointSignal) WaitContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ch:
		return nil
	}
}

func (s *EndpointSignal) WaitTimeout(sec time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), sec)
	defer cancel()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ch:
		return nil
	}
}

func (s *EndpointSignal) Wait() {
	<-s.ch
}

func (s *EndpointSignal) Done() <-chan struct{} {
	return s.ch
}

func (s *EndpointSignal) ActiveContext(ctx context.Context) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	// An endpoint is a one-shot decision. Closing the channel turns the signal
	// into a latch: every current and future waiter observes the same decision,
	// and releasing before a waiter exists cannot strand a sender goroutine.
	s.once.Do(func() { close(s.ch) })
}

func (s *EndpointSignal) ActiveAsyncContext(ctx context.Context) {
	// Kept for API compatibility. Activation is now non-blocking by design, so
	// creating a goroutine here would only reintroduce a leak opportunity.
	s.ActiveContext(ctx)
}
