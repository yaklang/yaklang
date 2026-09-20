package utils

import (
	"sync/atomic"
	"testing"
	"time"
)

// A burst must invoke only its last callback. Receive the result instead of
// sleeping through hundreds of calls and then reading unsynchronized state.
func assertDebounceBurst(t *testing.T, debounce func(func()), count int) {
	t.Helper()
	called := make(chan int, count)
	for i := 0; i < count; i++ {
		index := i
		debounce(func() { called <- index })
	}
	select {
	case index := <-called:
		if index != count-1 {
			t.Fatalf("executed callback %d, want the last callback %d", index, count-1)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("debounced callback did not execute")
	}
	// Observe two further debounce intervals to catch duplicate/uncancelled
	// timers; this wait tests absence, rather than guessing when work is done.
	select {
	case index := <-called:
		t.Fatalf("extra debounced callback: %d", index)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestNewDebounce(t *testing.T) {
	assertDebounceBurst(t, NewDebounce(0.1), 240)
}

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

func TestNewThrottleEx(t *testing.T) {
	wait := 0.1

	// 测试 lead=true, tail=false (立即执行，然后节流)
	t.Run("lead=true,tail=false", func(t *testing.T) {
		throttle := NewThrottleEx(wait, true, false)
		var counter int32

		// 快速连续调用，应该只执行第一次
		for i := 0; i < 5; i++ {
			throttle(func() { atomic.AddInt32(&counter, 1) })
		}

		time.Sleep(FloatSecondDuration(wait * 1.5))
		if atomic.LoadInt32(&counter) != 1 {
			t.Errorf("Expected 1 call, got %d", atomic.LoadInt32(&counter))
		}

		// 再次调用，应该执行
		throttle(func() { atomic.AddInt32(&counter, 1) })
		time.Sleep(FloatSecondDuration(wait * 0.5))
		if atomic.LoadInt32(&counter) != 2 {
			t.Errorf("Expected 2 calls, got %d", atomic.LoadInt32(&counter))
		}
	})

	// 测试 lead=false, tail=true (只在最后执行)
	t.Run("lead=false,tail=true", func(t *testing.T) {
		throttle := NewThrottleEx(wait, false, true)
		var counter int32

		// 快速连续调用，只在最后执行一次
		for i := 0; i < 5; i++ {
			throttle(func() { atomic.AddInt32(&counter, 1) })
		}

		time.Sleep(FloatSecondDuration(wait * 1.5))
		if atomic.LoadInt32(&counter) != 1 {
			t.Errorf("Expected 1 call, got %d", atomic.LoadInt32(&counter))
		}
	})

	// 测试 lead=true, tail=true (立即执行第一次，最后执行一次)
	t.Run("lead=true,tail=true", func(t *testing.T) {
		throttle := NewThrottleEx(wait, true, true)
		var counter int32

		// 快速连续调用，应该立即执行一次，最后再执行一次
		for i := 0; i < 5; i++ {
			throttle(func() { atomic.AddInt32(&counter, 1) })
		}

		time.Sleep(FloatSecondDuration(wait * 1.5))
		if atomic.LoadInt32(&counter) != 2 {
			t.Errorf("Expected 2 calls, got %d", atomic.LoadInt32(&counter))
		}
	})

	// 测试 lead=false, tail=false (不执行任何函数)
	t.Run("lead=false,tail=false", func(t *testing.T) {
		throttle := NewThrottleEx(wait, false, false)
		var counter int32

		for i := 0; i < 5; i++ {
			throttle(func() { atomic.AddInt32(&counter, 1) })
		}

		time.Sleep(FloatSecondDuration(wait * 1.5))
		if atomic.LoadInt32(&counter) != 0 {
			t.Errorf("Expected 0 calls, got %d", atomic.LoadInt32(&counter))
		}
	})
}

func TestNewDebounceEx(t *testing.T) {
	wait := 0.1

	// 测试 lead=false, tail=true (标准防抖)
	t.Run("lead=false,tail=true", func(t *testing.T) {
		debounce := NewDebounceEx(wait, false, true)
		var counter int32

		// 快速连续调用，只在最后执行一次
		for i := 0; i < 5; i++ {
			debounce(func() { atomic.AddInt32(&counter, 1) })
			time.Sleep(FloatSecondDuration(wait * 0.1))
		}

		time.Sleep(FloatSecondDuration(wait * 1.5))
		if atomic.LoadInt32(&counter) != 1 {
			t.Errorf("Expected 1 call, got %d", atomic.LoadInt32(&counter))
		}
	})

	// 测试 lead=true, tail=false (只在第一次立即执行)
	t.Run("lead=true,tail=false", func(t *testing.T) {
		debounce := NewDebounceEx(wait, true, false)
		var counter int32

		// 第一次调用立即执行
		debounce(func() { atomic.AddInt32(&counter, 1) })
		time.Sleep(FloatSecondDuration(wait * 0.5))
		if atomic.LoadInt32(&counter) != 1 {
			t.Errorf("Expected 1 call, got %d", atomic.LoadInt32(&counter))
		}

		// 后续调用不执行
		for i := 0; i < 3; i++ {
			debounce(func() { atomic.AddInt32(&counter, 1) })
			time.Sleep(FloatSecondDuration(wait * 0.1))
		}

		time.Sleep(FloatSecondDuration(wait * 1.5))
		if atomic.LoadInt32(&counter) != 1 {
			t.Errorf("Expected 1 call, got %d", atomic.LoadInt32(&counter))
		}
	})

	// 测试 lead=true, tail=true (第一次立即执行，最后也执行)
	t.Run("lead=true,tail=true", func(t *testing.T) {
		debounce := NewDebounceEx(wait, true, true)
		var counter int32

		// 第一次调用立即执行
		debounce(func() { atomic.AddInt32(&counter, 1) })
		time.Sleep(FloatSecondDuration(wait * 0.5))
		if atomic.LoadInt32(&counter) != 1 {
			t.Errorf("Expected 1 call, got %d", atomic.LoadInt32(&counter))
		}

		// 后续快速调用，最后一次也执行
		for i := 0; i < 3; i++ {
			debounce(func() { atomic.AddInt32(&counter, 1) })
			time.Sleep(FloatSecondDuration(wait * 0.1))
		}

		time.Sleep(FloatSecondDuration(wait * 1.5))
		if atomic.LoadInt32(&counter) != 2 {
			t.Errorf("Expected 2 calls, got %d", atomic.LoadInt32(&counter))
		}
	})

	// 测试 lead=false, tail=false (不执行任何函数)
	t.Run("lead=false,tail=false", func(t *testing.T) {
		debounce := NewDebounceEx(wait, false, false)
		var counter int32

		for i := 0; i < 5; i++ {
			debounce(func() { atomic.AddInt32(&counter, 1) })
			time.Sleep(FloatSecondDuration(wait * 0.1))
		}

		time.Sleep(FloatSecondDuration(wait * 1.5))
		if atomic.LoadInt32(&counter) != 0 {
			t.Errorf("Expected 0 calls, got %d", atomic.LoadInt32(&counter))
		}
	})
}

func TestNewDebounceExtended(t *testing.T) {
	debounce := NewDebounce(0.1)
	assertDebounceBurst(t, debounce, 10)
	// The same debouncer must accept a new burst after the first one completes.
	assertDebounceBurst(t, debounce, 10)
}

func TestNewDebounceResetsDeadline(t *testing.T) {
	const interval = 200 * time.Millisecond
	debounce := NewDebounce(interval.Seconds())
	called := make(chan int, 2)
	debounce(func() { called <- 0 })
	// A real gap is necessary here: a tight burst alone cannot distinguish
	// resetting the deadline from only replacing the pending callback.
	time.Sleep(50 * time.Millisecond)
	lastCall := time.Now()
	debounce(func() { called <- 1 })
	select {
	case index := <-called:
		if index != 1 {
			t.Fatalf("callback %d fired instead of the replacement", index)
		}
		if elapsed := time.Since(lastCall); elapsed < interval*9/10 {
			t.Fatalf("replacement fired before its debounce interval: %s", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("replacement callback did not execute")
	}
}

func TestNewThrottleExtended(t *testing.T) {
	var count int
	throttle := NewThrottle(0.2)
	var indexes []int

	// 快速连续调用
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		throttle(func() {
			count++
			indexes = append(indexes, i)
		})
	}

	time.Sleep(FloatSecondDuration(0.3))
	if count < 1 {
		t.Fatalf("Expected at least 1 call, got %d", count)
	}

	// 再次测试节流效果
	count = 0
	for i := 0; i < 5; i++ {
		throttle(func() {
			count++
		})
	}
	time.Sleep(FloatSecondDuration(0.1))
	if count != 1 {
		t.Fatalf("Expected 1 call during throttle period, got %d", count)
	}
}
