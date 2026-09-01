package utils

import (
	"math/rand"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
)

/* delay with range */
type DelayWaiter struct {
	max, min  int32
	nextDelay time.Duration
	mu        sync.Mutex
}

func abs(a, b int32) int32 {
	if a > b {
		return a - b
	}
	return b - a
}

func NewDelayWaiter(min int32, max int32) (*DelayWaiter, error) {
	if min > 0 && max > 0 {
		if max < min {
			return nil, errors.Errorf("min delay[%d/s] should be less than max delay[%d/s]", min, max)
		}
	}

	if min > 0 && max <= 0 {
		return nil, errors.Errorf("min: %d max: %d failed", min, max)
	}

	d := &DelayWaiter{
		min: min,
		max: max,
	}
	return d, nil
}

func (d *DelayWaiter) Wait() {
	d.mu.Lock()
	delay := d.nextDelay
	// 先计算下一次延迟再释放锁；sleep 在锁外进行（修复并发读写 nextDelay 的数据竞态）
	if abs(d.max, d.min) > 0 {
		d.nextDelay = time.Duration(int(rand.Int31n(abs(d.max, d.min))+d.min) * int(time.Second))
	} else {
		d.nextDelay = time.Duration(int(d.min) * int(time.Second))
	}
	d.mu.Unlock()
	time.Sleep(delay)
}

func (d *DelayWaiter) WaitWithProbabilityPercent(raw float64) {
	if raw < 0 || raw > 1 {
		log.Errorf("failed to use delay probability percent: %v", raw)
	} else {
		if rand.Float64() > raw {
			return
		}
	}

	d.Wait()
}

// GetDelayRange 返回延迟配置范围（秒转时长）。
func (d *DelayWaiter) GetDelayRange() (min, max time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return time.Duration(d.min) * time.Second, time.Duration(d.max) * time.Second
}
