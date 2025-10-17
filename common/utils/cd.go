package utils

import (
	"context"
	"sync/atomic"
	"time"
)

type CoolDown struct {
	du     int64
	ctx    context.Context
	cancel func()

	coreChan chan interface{}
}

func NewCoolDownContext(d time.Duration, ctx context.Context) *CoolDown {
	cd := &CoolDown{du: int64(d)}
	cd.ctx, cd.cancel = context.WithCancel(ctx)
	cd.coreChan = make(chan interface{}, 1)

	cd.coreChan <- nil

	go func() {
		exitForCtx := func() {
			select {
			case <-cd.ctx.Done():
				return
			default:
			}
		}

		for {
			duration := time.Duration(atomic.LoadInt64(&cd.du))
			time.Sleep(duration)
			exitForCtx()
			select {
			case cd.coreChan <- nil:
			case <-cd.ctx.Done():
				return
			}
		}
	}()
	return cd
}

func NewCoolDown(d time.Duration) *CoolDown {
	return NewCoolDownContext(d, context.Background())
}

func Spinlock(t float64, h func() bool) error {
	ctx := TimeoutContextSeconds(t)
	for {
		select {
		case <-ctx.Done():
			return Error("Spinlock timeout")
		default:
			if h() {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (c *CoolDown) Reset(d time.Duration) {
	atomic.StoreInt64(&c.du, int64(d))
}

func (c *CoolDown) Do(f func()) {
	select {
	case <-c.coreChan:
		f()
	default:
	}
}

func (c *CoolDown) DoOr(f func(), fallback func()) {
	select {
	case <-c.coreChan:
		f()
	default:
		fallback()
	}
}
