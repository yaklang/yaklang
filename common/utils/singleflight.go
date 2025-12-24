package utils

import (
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

