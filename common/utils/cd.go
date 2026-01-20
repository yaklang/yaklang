package utils

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type CoolDown struct {
	du     int64
	ctx    context.Context
	cancel func()

	coreChan chan struct{}

	triggerChan chan struct{}
	resetChan   chan struct{}

	stopOnce sync.Once
}

func NewCoolDownContext(d time.Duration, ctx context.Context) *CoolDown {
	cd := &CoolDown{du: int64(d)}
	cd.ctx, cd.cancel = context.WithCancel(ctx)
	cd.coreChan = make(chan struct{}, 1)
	cd.triggerChan = make(chan struct{}, 1)
	cd.resetChan = make(chan struct{}, 1)

	cd.coreChan <- struct{}{}

	go func() {
		var timer *time.Timer
		var timerChan <-chan time.Time

		stopTimer := func() {
			if timer == nil {
				return
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer = nil
			timerChan = nil
		}

		restartTimer := func() {
			stopTimer()

			duration := time.Duration(atomic.LoadInt64(&cd.du))
			if duration <= 0 {
				select {
				case <-cd.ctx.Done():
					return
				default:
				}
				select {
				case cd.coreChan <- struct{}{}:
				default:
				}
				return
			}

			timer = time.NewTimer(duration)
			timerChan = timer.C
		}

		for {
			select {
			case <-cd.ctx.Done():
				stopTimer()
				return
			case <-cd.triggerChan:
				restartTimer()
			case <-cd.resetChan:
				if timer != nil {
					restartTimer()
				}
			case <-timerChan:
				stopTimer()
				select {
				case cd.coreChan <- struct{}{}:
				default:
				}
			}
		}
	}()
	return cd
}

func NewCoolDown(d time.Duration) *CoolDown {
	return NewCoolDownContext(d, context.Background())
}

func (c *CoolDown) Close() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
	})
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
	select {
	case c.resetChan <- struct{}{}:
	default:
	}
}

func (c *CoolDown) Do(f func()) {
	select {
	case <-c.coreChan:
		f()
		select {
		case c.triggerChan <- struct{}{}:
		default:
		}
	default:
	}
}

func (c *CoolDown) DoOr(f func(), fallback func()) {
	select {
	case <-c.coreChan:
		f()
		select {
		case c.triggerChan <- struct{}{}:
		default:
		}
	default:
		fallback()
	}
}
