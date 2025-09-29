package utils

import (
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
			f()
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
			f()
			timer = time.AfterFunc(FloatSecondDuration(wait), func() {
				timer = nil
			})
		}
	}
}

// NewThrottleEx 创建一个扩展的节流函数
// wait: 节流时间间隔
// lead: 是否在第一次调用时立即执行
// tail: 是否在最后一次调用后延迟执行
func NewThrottleEx(wait float64, lead bool, tail bool) func(f func()) {
	var timer *time.Timer
	var mu sync.Mutex
	var pending bool

	return func(f func()) {
		mu.Lock()
		defer mu.Unlock()

		if timer == nil {
			// 第一次调用或定时器已过期
			if lead {
				f()
			}
			timer = time.AfterFunc(FloatSecondDuration(wait), func() {
				mu.Lock()
				defer mu.Unlock()
				if tail && pending {
					f()
					pending = false
				}
				timer = nil
			})
		} else {
			// 在节流期间，标记有挂起的调用
			if tail {
				pending = true
			}
		}
	}
}

// NewDebounceEx 创建一个扩展的防抖函数
// wait: 防抖时间间隔
// lead: 是否在第一次调用时立即执行
// tail: 是否在最后一次调用后延迟执行
func NewDebounceEx(wait float64, lead bool, tail bool) func(f func()) {
	var timer *time.Timer
	var mu sync.Mutex
	var leadingCallExecuted bool

	return func(f func()) {
		mu.Lock()
		defer mu.Unlock()

		shouldCall := timer == nil

		if timer != nil {
			timer.Stop()
		}

		if shouldCall && lead && !leadingCallExecuted {
			f()
			leadingCallExecuted = true
		}

		timer = time.AfterFunc(FloatSecondDuration(wait), func() {
			mu.Lock()
			defer mu.Unlock()
			if tail {
				f()
			}
			timer = nil
			leadingCallExecuted = false
		})
	}
}
