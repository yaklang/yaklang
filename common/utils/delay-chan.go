package utils

import (
	"github.com/pkg/errors"
	"math/rand"
	"time"
	"github.com/yaklang/yaklang/common/log"
)

/* delay with range */
type DelayWaiter struct {
	max, min  int32
	nextDelay time.Duration
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
	time.Sleep(d.nextDelay)
	if abs(d.max, d.min) > 0 {
		d.nextDelay = time.Duration(int(rand.Int31n(abs(d.max, d.min))+d.min) * int(time.Second))
	} else {
		d.nextDelay = time.Duration(int(d.min) * int(time.Second))
	}
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
