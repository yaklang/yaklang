package aid

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"time"
)

type signal struct {
	ch chan struct{}
}

func newSignal() *signal {
	return &signal{
		ch: make(chan struct{}),
	}
}

func (s *signal) WaitContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ch:
		return nil
	}
}

func (s *signal) WaitTimeout(sec time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), sec)
	defer cancel()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ch:
		return nil
	}
}

func (s *signal) Wait() {
	<-s.ch
}

func (s *signal) activeAsyncContext(ctx context.Context) {
	active := make(chan struct{})
	go func() {
		s.activeContext(active, ctx)
	}()
	<-active
}

func (s *signal) ActiveContext(ctx context.Context) {
	s.activeContext(nil, ctx)
}

func (s *signal) ActiveAsyncContext(ctx context.Context) {
	s.activeAsyncContext(ctx)
}

func (c *signal) activeContext(started chan struct{}, ctx context.Context) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("aid signal active async error, %v", err)
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
