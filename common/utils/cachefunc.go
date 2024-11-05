package utils

import (
	"sync"
	"time"
)

type cacheEntry[T any] struct {
    value     T
    err       error
    timestamp time.Time
}

func CacheFunc[T any](duration time.Duration, f func() (T, error)) func() (T, error) {
    var mu sync.RWMutex
    var entry cacheEntry[T]

    // 初始化缓存
    value, err := tryGetValue(f)
    entry = cacheEntry[T]{
        value:     value,
        err:       err,
        timestamp: time.Now(),
    }

    return func() (T, error) {
		// 1. 尝试读取缓存
        mu.RLock()
        if time.Since(entry.timestamp) < duration {
            defer mu.RUnlock()
            return entry.value, entry.err
        }
        mu.RUnlock()

        // 2. 缓存过期，需要更新缓存
        mu.Lock()
        defer mu.Unlock()

        // 双重检查，避免并发更新
        if time.Since(entry.timestamp) < duration {
            return entry.value, entry.err
        }
		// 3. 更新缓存
        value, err := tryGetValue(f)
        entry = cacheEntry[T]{
            value:     value,
            err:       err,
            timestamp: time.Now(),
        }
        return entry.value, entry.err
    }
}

// 尝试获取值，最多重试3次
func tryGetValue[T any](f func() (T, error)) (T, error) {
    var lastErr error
    for i := 0; i < 3; i++ {
        if value, err := f(); err == nil {
            return value, nil
        } else {
            lastErr = err
            time.Sleep(time.Millisecond * 100) // 重试间隔
        }
    }
    var zero T
    return zero, Errorf("all retry attempts failed: %v", lastErr)
}
