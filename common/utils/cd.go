package utils

import (
	"context"
	"time"
)

type CoolDown struct {
	du     time.Duration
	ctx    context.Context
	cancel func()

	coreChan chan interface{}
}

func NewCoolDownContext(d time.Duration, ctx context.Context) *CoolDown {
	cd := &CoolDown{du: d}
	cd.ctx, cd.cancel = context.WithCancel(ctx)
	cd.coreChan = make(chan interface{}, 0)

	go func() {
		exitForCtx := func() {
			select {
			case <-cd.ctx.Done():
				return
			default:
			}
		}

		for {
			exitForCtx()
			cd.coreChan <- nil
			time.Sleep(cd.du)

			exitForCtx()
		}
	}()
	return cd
}

func NewCoolDown(d time.Duration) *CoolDown {
	return NewCoolDownContext(d, context.Background())
}

func (c *CoolDown) Reset(d time.Duration) {
	c.du = d
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
