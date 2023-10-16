package utils

import (
	"github.com/yaklang/yaklang/common/log"
	"sync"
	"time"
)

func NewDebounce(wait float64) func(f func()) {
	var timer *time.Timer
	var mu sync.Mutex

	return func(f func()) {
		mu.Lock()
		defer mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(FloatSecondDuration(wait), func() {
			log.Info("start to call")
			f()
			log.Info("end to call")
		})
	}
}

func NewThrottle(wait float64) func(f func()) {
	var timer *time.Timer
	var mu sync.Mutex
	return func(f func()) {
		mu.Lock()
		defer mu.Unlock()

		if timer == nil {
			timer = time.AfterFunc(FloatSecondDuration(wait), func() {
				f()
				timer = nil
			})
		}
	}
}
