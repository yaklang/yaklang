package utils

import (
	"context"
	"fmt"

	"golang.org/x/sync/singleflight"
)

// SingleFlightCache wraps singleflight with cache support
type SingleFlightCache[K comparable, V any] struct {
	sf    singleflight.Group
	cache *CacheExWithKey[K, V]
}

// NewSingleFlightCache creates a new SingleFlightCache. If cache is nil, only single-flight behavior is provided.
func NewSingleFlightCache[K comparable, V any](cache *CacheExWithKey[K, V]) *SingleFlightCache[K, V] {
	return &SingleFlightCache[K, V]{
		sf:    singleflight.Group{},
		cache: cache,
	}
}

// Do executes the loader function only once for the same key, even with concurrent requests.
func (s *SingleFlightCache[K, V]) Do(key K, loader func() (V, error)) (V, error) {
	if s.cache != nil {
		if value, ok := s.cache.Get(key); ok {
			return value, nil
		}
	}

	keyStr := fmt.Sprintf("%v", key)
	result, err, _ := s.sf.Do(keyStr, func() (interface{}, error) {
		if s.cache != nil {
			if value, ok := s.cache.Get(key); ok {
				return value, nil
			}
		}

		data, err := loader()
		if err == nil && s.cache != nil {
			s.cache.Set(key, data)
		}

		return data, err
	})

	if err != nil {
		var zero V
		return zero, err
	}

	if res, ok := result.(V); ok {
		return res, nil
	}

	var zero V
	return zero, Errorf("unexpected result type from singleflight")
}

// DoChan is the context-aware variant of Do. The loader still runs once per
// key, but a caller can stop waiting when ctx is done; the flight itself keeps
// running under the loader's own cancellation and a successful result is still
// cached. A panicking loader is reported as an error to every caller instead of
// crashing the process from singleflight's internal goroutine.
func (s *SingleFlightCache[K, V]) DoChan(ctx context.Context, key K, loader func() (V, error)) (V, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.cache != nil {
		if value, ok := s.cache.Get(key); ok {
			return value, nil
		}
	}

	keyStr := fmt.Sprintf("%v", key)
	result := s.sf.DoChan(keyStr, func() (interface{}, error) {
		if s.cache != nil {
			if value, ok := s.cache.Get(key); ok {
				return value, nil
			}
		}

		data, err := safeLoad(loader)
		if err == nil && s.cache != nil {
			s.cache.Set(key, data)
		}

		return data, err
	})

	select {
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	case ret := <-result:
		if ret.Err != nil {
			var zero V
			return zero, ret.Err
		}
		if res, ok := ret.Val.(V); ok {
			return res, nil
		}
		var zero V
		return zero, Errorf("unexpected result type from singleflight")
	}
}

// safeLoad converts a loader panic into the error every caller observes.
func safeLoad[V any](loader func() (V, error)) (value V, err error) {
	defer func() {
		if r := recover(); r != nil {
			var zero V
			value = zero
			err = Errorf("singleflight: panic: %v", r)
		}
	}()
	return loader()
}
