package utils

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

// CacheFunc do cache and retry
// It's used for unstable func.
func CacheFunc[T any](t time.Duration, f func() (T, error)) func() (T, error) {
	var (
		mutex    sync.Mutex
		timer    = time.NewTimer(t)
		cache    T
		err      error
		updating bool // 是否正在更新缓存
	)

	updateCache := func() {
		mutex.Lock()
		if updating {
			mutex.Unlock()
			return
		}
		updating = true
		mutex.Unlock()

		const maxAttempts = 3
		var lastErr error
		for i := 0; i < maxAttempts; i++ {
			newCache, newErr := f()
			if newErr == nil {
				mutex.Lock()
				cache, err = newCache, nil
				updating = false
				mutex.Unlock()
				break
			} else {
				log.Errorf("cache update attempt %d failed: %s", i+1, newErr)
				lastErr = newErr
			}
		}

		mutex.Lock()
		if lastErr != nil {
			err = lastErr
		}
		updating = false
		mutex.Unlock()

		timer.Reset(t) // Reset timer after update attempt
	}

	timer = time.AfterFunc(t, updateCache)

	// 首次填充缓存
	updateCache()

	return func() (T, error) {
		mutex.Lock()
		defer mutex.Unlock()
		return cache, err
	}
}
