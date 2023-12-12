package utils

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"math/rand"
	"time"
)

/* delay with range */
type FloatSecondsDelayWaiter struct {
	max, min  float64
	nextDelay time.Duration
}

func absRange(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

func NewFloatSecondsDelayWaiterSingle(min float64) (*FloatSecondsDelayWaiter, error) {
	return NewFloatSecondsDelayWaiter(min, min)
}

func NewFloatSecondsDelayWaiter(min, max float64) (*FloatSecondsDelayWaiter, error) {
	if min > 0 && max > 0 {
		if max < min {
			return nil, errors.Errorf("min delay[%v/s] should be less than max delay[%v/s]", min, max)
		}
	}

	if min > 0 && max <= 0 {
		return nil, errors.Errorf("min: %v max: %v failed", min, max)
	}
	var nextDelay time.Duration
	if absRange(max, min) > 0 {
		randomFloat64 := rand.Float64() * (absRange(max, min))
		nextDelay = time.Duration(int(1000*(randomFloat64+min)) * int(time.Millisecond))
	} else {
		nextDelay = time.Duration(int(1000*min) * int(time.Millisecond))
	}

	d := &FloatSecondsDelayWaiter{
		min:       min,
		max:       max,
		nextDelay: nextDelay,
	}

	return d, nil
}

func (d *FloatSecondsDelayWaiter) Wait() {
	if d == nil {
		return
	}

	time.Sleep(d.nextDelay)
	if absRange(d.max, d.min) > 0 {
		randomFloat64 := rand.Float64() * (absRange(d.max, d.min))
		d.nextDelay = time.Duration(int(1000*(randomFloat64+d.min)) * int(time.Millisecond))
	} else {
		d.nextDelay = time.Duration(int(1000*d.min) * int(time.Millisecond))
	}
	log.Debugf("next delayer: %v", d.nextDelay.String())
}

func (d *FloatSecondsDelayWaiter) WaitWithProbabilityPercent(raw float64) {
	if raw < 0 || raw > 1 {
		log.Errorf("failed to use delay probability percent: %v", raw)
	} else {
		if rand.Float64() > raw {
			return
		}
	}

	d.Wait()
}
