package utils

import "time"

// CacheFunc do cache and retry
// It's used for unstable func.
func CacheFunc[T any](t time.Duration, f func() (T, error)) func() (T, error) {
	timer := time.NewTimer(t)
	var cache T
	var err error
	return func() (T, error) {
		select {
		case <-timer.C:
			for i := 0; i < 3; i++ {
				cache, err = f()
				if err == nil {
					return cache, nil
				}
			}
			return cache, err
		default:
			return cache, nil
		}
	}
}
