package utils

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheFunc(t *testing.T) {
	tests := []struct {
		name          string
		duration      time.Duration
		function      func() (int, error)
		expectedValue int
		expectedError error
	}{
		{
			name:     "successful caching",
			duration: 100 * time.Millisecond,
			function: func() (int, error) {
				return 42, nil
			},
			expectedValue: 42,
			expectedError: nil,
		},
		{
			name:     "function returns error",
			duration: 100 * time.Millisecond,
			function: func() (int, error) {
				return 0, errors.New("failure")
			},
			expectedValue: 0,
			expectedError: Errorf("all retry attempts failed: %v", errors.New("failure")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cachedFunc := CacheFunc[int](tt.duration, tt.function)
			value, err := cachedFunc()

			if value != tt.expectedValue {
				t.Errorf("CacheFunc() = %v, want %v", value, tt.expectedValue)
			}

			if (err != nil && tt.expectedError == nil) || (err == nil && tt.expectedError != nil) ||
				(err != nil && tt.expectedError != nil && err.Error() != tt.expectedError.Error()) {
				t.Errorf("CacheFunc() error = %v, wantErr %v", err, tt.expectedError)
			}
		})
	}
}

func TestCacheFuncConcurrent(t *testing.T) {
	const numGoroutines = 10 // 并发协程的数量
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// 使用一个简单的函数，返回固定的值
	cachedFunc := CacheFunc[int](500*time.Millisecond, func() (int, error) {
		return 42, nil
	})

	// 创建多个协程同时调用缓存函数
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			value, err := cachedFunc()
			if value != 42 || err != nil {
				t.Errorf("CacheFunc() = %v, %v, want %v, %v", value, err, 42, nil)
			}
		}()
	}

	wg.Wait() // 等待所有协程完成
}

func TestCacheFuncConcurrentWithErrors(t *testing.T) {
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// 使用一个可能返回错误的函数
	cachedFunc := CacheFunc[int](500*time.Millisecond, func() (int, error) {
		return 0, errors.New("failure")
	})

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			value, err := cachedFunc()
			if value != 0 || err == nil || err.Error() != Errorf("all retry attempts failed: %v", errors.New("failure")).Error() {
				t.Errorf("CacheFunc() = %v, %v, want %v, %v", value, err, 0, "failure")
			}
		}()
	}

	wg.Wait()
}

func TestCacheFuncCachingAndTimeout(t *testing.T) {
	var count int32 // 用来记录函数调用次数
	cachedFunc := CacheFunc[int](100*time.Millisecond, func() (int, error) {
		atomic.AddInt32(&count, 1)
		return 42, nil
	})

	// 第一次调用，应该执行函数
	value, err := cachedFunc()
	if value != 42 || err != nil {
		t.Errorf("CacheFunc() = %v, %v, want %v, %v", value, err, 42, nil)
	}
	if count != 1 {
		t.Errorf("Function should have been called once, but was called %d times", count)
	}

	// 短时间内再次调用，不应该执行函数
	value, err = cachedFunc()
	if value != 42 || err != nil {
		t.Errorf("CacheFunc() = %v, %v, want %v, %v", value, err, 42, nil)
	}
	if count != 1 {
		t.Errorf("Function should have been called once, but was called %d times", count)
	}

	// 等待超过缓存时间，再次调用应该执行函数
	time.Sleep(150 * time.Millisecond)
	value, err = cachedFunc()
	if value != 42 || err != nil {
		t.Errorf("CacheFunc() = %v, %v, want %v, %v", value, err, 42, nil)
	}
	if count != 2 {
		t.Errorf("Function should have been called twice, but was called %d times", count)
	}
}
