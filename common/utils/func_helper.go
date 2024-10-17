package utils

import (
	"sync"
	"time"
)

//func NewDebounce(wait float64) func(f func()) {
//	var timer *time.Timer
//	var mu sync.Mutex
//
//	return func(f func()) {
//		mu.Lock()
//		defer mu.Unlock()
//		if timer != nil {
//			timer.Stop()
//		}
//		timer = time.AfterFunc(FloatSecondDuration(wait), func() {
//			f()
//		})
//	}
//}

func NewThrottle(wait float64) func(f func()) {
	var timer *time.Timer
	var mu sync.Mutex
	return func(f func()) {
		mu.Lock()
		defer mu.Unlock()

		if timer == nil {
			f()
			timer = time.AfterFunc(FloatSecondDuration(wait), func() {
				timer = nil
			})
		}
	}
}
