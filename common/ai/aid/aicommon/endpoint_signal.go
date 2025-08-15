package aicommon

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"time"
)

type EndpointSignal struct {
	ch chan struct{}
}

func NewEndpointSignal() *EndpointSignal {
	return &EndpointSignal{
		ch: make(chan struct{}),
	}
}

func (s *EndpointSignal) WaitContext(ctx context.Context) error {
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

func (s *EndpointSignal) activeAsyncContext(ctx context.Context) {
	active := make(chan struct{})
	go func() {
		s.activeContext(active, ctx)
	}()
	<-active
}

func (s *EndpointSignal) ActiveContext(ctx context.Context) {
	s.activeContext(nil, ctx)
}

func (s *EndpointSignal) ActiveAsyncContext(ctx context.Context) {
	s.activeAsyncContext(ctx)
}

func (c *EndpointSignal) activeContext(started chan struct{}, ctx context.Context) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("aid EndpointSignal active async error, %v", err)
		}
	}()

	if started != nil {
		started <- struct{}{}
	}

	select {
	case <-ctx.Done():
		return
	case c.ch <- struct{}{}:
	}
}
