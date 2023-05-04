package utils

import (
	"context"
	"time"
)

type LimitRate struct {
	ctx      context.Context
	cancel   context.CancelFunc
	ch       chan interface{}
	duration time.Duration
}

func (l *LimitRate) WaitUntilNextSync() {
	select {
	case <-l.ch:
	case <-l.ctx.Done():
	}
}

func (l *LimitRate) WaitUntilNextAsync() {
	l.WaitUntilNextAsyncWithFallback(nil)
}

func (l *LimitRate) WaitUntilNextAsyncWithFallback(f func()) {
	select {
	case <-l.ch:
	default:
		if f != nil {
			f()
		}
	}
}

func NewLimitRate(d time.Duration) *LimitRate {
	ctx, cancel := context.WithCancel(context.Background())
	l := &LimitRate{
		ctx: ctx, cancel: cancel,
		ch:       make(chan interface{}),
		duration: d,
	}

	go func() {
		for {
			select {
			case l.ch <- nil:
				time.Sleep(l.duration)
			case <-ctx.Done():
				return
			}
		}
	}()
	return l
}
