package utils

import (
	"time"
)

type CacheWithKey[U comparable, T any] struct {
	*CacheExWithKey[U, T]
	expireCallback expireCallbackWithoutReason[U, T]
}

type Cache[T any] struct {
	*CacheWithKey[string, T]
}

// ExpireCallback is used as a callback on item expiration or when notifying of an item new to the cache
type expireCallbackWithoutReason[U comparable, T any] func(key U, value T)

// SetExpirationCallback sets a callback that will be called when an item expires
func (cache *CacheWithKey[U, T]) SetExpirationCallback(callback expireCallbackWithoutReason[U, T]) {
	cache.expireCallback = callback
}

// GetOrLoad attempts to retrieve data from the cache for the given key.
// If the data is not present, it will execute the dataLoader function to load it.
// This method provides single-flight behavior: for a given key, the dataLoader function
// is executed only once concurrently. Multiple concurrent requests for the same key
// will wait for the single loading operation to complete and then receive its result.
func (cache *CacheWithKey[U, T]) GetOrLoad(key U, dataLoader func() (T, error)) (T, error) {
	return cache.CacheExWithKey.GetOrLoad(key, dataLoader)
}

// NewTTLCache is a helper to create instance of the Cache struct
func NewTTLCache[T any](ttls ...time.Duration) *Cache[T] {
	return &Cache[T]{
		CacheWithKey: NewTTLCacheWithKey[string, T](ttls...),
	}
}

// NewTTLCacheWithKey is a helper to create instance of the CacheWithKey struct, allow set Key and Value
func NewTTLCacheWithKey[U comparable, T any](ttls ...time.Duration) *CacheWithKey[U, T] {
	ret := &CacheWithKey[U, T]{
		CacheExWithKey: NewCacheExWithKey[U, T](WithCacheTTL(ttls...)),
	}
	ret.CacheExWithKey.SetExpirationCallback(func(key U, value T, reason EvictionReason) {
		if reason != EvictionReasonExpired {
			return
		}
		if ret.expireCallback != nil {
			ret.expireCallback(key, value)
		}
	})
	return ret
}

func NewLRUCache[T any](capacity uint64) *Cache[T] {
	return &Cache[T]{
		CacheWithKey: NewLRUCacheWithKey[string, T](capacity),
	}
}

func NewLRUCacheWithKey[U comparable, T any](capacity uint64) *CacheWithKey[U, T] {
	ret := &CacheWithKey[U, T]{
		CacheExWithKey: NewCacheExWithKey[U, T](WithCacheCapacity(capacity)),
	}
	ret.CacheExWithKey.SetExpirationCallback(func(key U, value T, reason EvictionReason) {
		if reason != EvictionReasonCapacityReached {
			return
		}
		if ret.expireCallback != nil {
			ret.expireCallback(key, value)
		}
	})

	return ret
}
