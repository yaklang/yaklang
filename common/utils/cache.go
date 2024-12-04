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
