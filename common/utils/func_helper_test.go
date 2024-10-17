package utils

import (
	"sync/atomic"
	"testing"
	"time"
)

//func TestNewDebounce(t *testing.T) {
//	var count int
//	debounceCaller := NewDebounce(1)
//	lastIndex := 0
//	for i := 0; i < 240; i++ {
//		time.Sleep(10 * time.Millisecond)
//		debounceCaller(func() {
//			count++
//			lastIndex = i
//			t.Log("debounce")
//		})
//	}
//	time.Sleep(FloatSecondDuration(1.1))
//	if count != 1 {
//		t.Fatal("debounce failed")
//	}
//	if lastIndex != 240 {
//		spew.Dump(lastIndex)
//		t.Fatal("debounce failed")
//	}
//}

//	func TestNewThrottle(t *testing.T) {
//		var count int
//		caller := NewThrottle(1)
//		var indexes []int
//		for i := 0; i < 240; i++ {
//			time.Sleep(10 * time.Millisecond)
//			caller(func() {
//				count++
//				t.Log("throttle")
//				spew.Dump(i)
//				indexes = append(indexes, i)
//			})
//		}
//		spew.Dump(count)
//		if count != 2 {
//			t.Fatal("throttle failed")
//		}
//
//		if indexes[0] < 80 {
//			t.Fatal("throttle failed")
//		}
//		spew.Dump(indexes)
//	}
func TestNewThrottleAi(t *testing.T) {
	// 设置等待时间为0.1秒
	wait := 0.1
	throttle := NewThrottle(wait)

	// 用于记录函数调用次数的原子计数器
	var counter int32

	// 创建一个被截流的函数
	throttledFunc := func() {
		atomic.AddInt32(&counter, 1)
	}

	// 测试场景1: 快速连续调用
	for i := 0; i < 10; i++ {
		throttle(throttledFunc)
	}

	// 等待略长于等待时间
	time.Sleep(FloatSecondDuration(wait * 1.5))

	if atomic.LoadInt32(&counter) != 1 {
		t.Errorf("Expected 1 call, got %d", atomic.LoadInt32(&counter))
	}

	// 测试场景2: 间隔调用
	throttle(throttledFunc)
	time.Sleep(FloatSecondDuration(wait * 1.5))
	throttle(throttledFunc)
	time.Sleep(FloatSecondDuration(wait * 1.5))

	if atomic.LoadInt32(&counter) != 3 {
		t.Errorf("Expected 3 calls, got %d", atomic.LoadInt32(&counter))
	}

	// 测试场景3: 在等待时间内调用
	throttle(throttledFunc)
	time.Sleep(FloatSecondDuration(wait * 0.5))
	throttle(throttledFunc)
	time.Sleep(FloatSecondDuration(wait * 1.5))

	if atomic.LoadInt32(&counter) != 4 {
		t.Errorf("Expected 4 calls, got %d", atomic.LoadInt32(&counter))
	}
}
