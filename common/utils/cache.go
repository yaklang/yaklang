package utils

import (
	"time"

	"github.com/jellydator/ttlcache/v3"
)

// CacheWithKey is a synchronized expiring cache backed by the shared
// non-generic cacheExCore. Holding core directly instead of embedding
// *CacheExWithKey avoids duplicating the entire method set as value- and
// pointer-receiver promoted wrappers on every generic instantiation.
type CacheWithKey[U comparable, T any] struct {
	core        *cacheExCore
	evictReason EvictionReason
}

// Cache is a string-keyed expiring cache backed by cacheExCore directly.
type Cache[T any] struct {
	core        *cacheExCore
	evictReason EvictionReason
}

// ExpireCallback is used as a callback on item expiration or when notifying of an item new to the cache
type expireCallbackWithoutReason[U comparable, T any] func(key U, value T)

func (cache *CacheWithKey[U, T]) Close()                               { cache.core.close() }
func (cache *CacheWithKey[U, T]) Set(key U, value T)                   { cache.core.set(key, value) }
func (cache *CacheWithKey[U, T]) SetWithTTL(k U, v T, d time.Duration) { cache.core.setWithTTL(k, v, d) }
func (cache *CacheWithKey[U, T]) Remove(key U) bool                    { return cache.core.remove(key) }
func (cache *CacheWithKey[U, T]) Delete(key U)                         { cache.core.deleteKey(key) }
func (cache *CacheWithKey[U, T]) DeleteAll()                           { cache.core.deleteAll() }
func (cache *CacheWithKey[U, T]) Count() int                           { return cache.core.count() }
func (cache *CacheWithKey[U, T]) SetTTL(ttl time.Duration)             { cache.core.setTTL(ttl) }
func (cache *CacheWithKey[U, T]) SkipTtlExtensionOnHit(value bool)     { cache.core.skipTtlExtensionOnHit(value) }
func (cache *CacheWithKey[U, T]) Purge()                               { cache.core.purge() }

func (cache *CacheWithKey[U, T]) Get(key U) (value T, exists bool) {
	raw, ok := cache.core.get(key)
	if !ok {
		return value, false
	}
	return castCacheValue[T](raw), true
}

func (cache *CacheWithKey[U, T]) GetAll() map[U]T {
	items := cache.core.items.Items()
	out := make(map[U]T, len(items))
	for k, item := range items {
		out[castCacheValue[U](k)] = castCacheValue[T](item.Value())
	}
	return out
}

func (cache *CacheWithKey[U, T]) ForEach(handler func(U, T)) {
	cache.core.items.Range(func(item *ttlcache.Item[any, any]) bool {
		handler(castCacheValue[U](item.Key()), castCacheValue[T](item.Value()))
		return true
	})
}

// SetExpirationCallback sets a callback that will be called when an item expires
func (cache *CacheWithKey[U, T]) SetExpirationCallback(callback expireCallbackWithoutReason[U, T]) {
	if callback == nil {
		cache.core.setExpirationCallback(nil)
		return
	}
	want := cache.evictReason
	cache.core.setExpirationCallback(func(key, value any, reason EvictionReason) {
		if want != 0 && reason != want {
			return
		}
		callback(castCacheValue[U](key), castCacheValue[T](value))
	})
}

func (cache *CacheWithKey[U, T]) SetNewItemCallback(callback itemCallback[U, T]) {
	if callback == nil {
		cache.core.setNewItemCallback(nil)
		return
	}
	cache.core.setNewItemCallback(func(key, value any) {
		callback(castCacheValue[U](key), castCacheValue[T](value))
	})
}

// GetOrLoad attempts to retrieve data from the cache for the given key.
// If the data is not present, it will execute the dataLoader function to load it.
// This method provides single-flight behavior: for a given key, the dataLoader function
// is executed only once concurrently. Multiple concurrent requests for the same key
// will wait for the single loading operation to complete and then receive its result.
func (cache *CacheWithKey[U, T]) GetOrLoad(key U, dataLoader func() (T, error)) (T, error) {
	raw, err := cache.core.getOrLoad(key, func() (any, error) {
		return dataLoader()
	})
	return castCacheValue[T](raw), err
}

func (cache *Cache[T]) Close()                                    { cache.core.close() }
func (cache *Cache[T]) Set(key string, value T)                   { cache.core.set(key, value) }
func (cache *Cache[T]) SetWithTTL(k string, v T, d time.Duration) { cache.core.setWithTTL(k, v, d) }
func (cache *Cache[T]) Remove(key string) bool                    { return cache.core.remove(key) }
func (cache *Cache[T]) Delete(key string)                         { cache.core.deleteKey(key) }
func (cache *Cache[T]) DeleteAll()                                { cache.core.deleteAll() }
func (cache *Cache[T]) Count() int                                { return cache.core.count() }
func (cache *Cache[T]) SetTTL(ttl time.Duration)                  { cache.core.setTTL(ttl) }
func (cache *Cache[T]) SkipTtlExtensionOnHit(value bool)          { cache.core.skipTtlExtensionOnHit(value) }
func (cache *Cache[T]) Purge()                                    { cache.core.purge() }

func (cache *Cache[T]) Get(key string) (value T, exists bool) {
	raw, ok := cache.core.get(key)
	if !ok {
		return value, false
	}
	return castCacheValue[T](raw), true
}

func (cache *Cache[T]) GetAll() map[string]T {
	items := cache.core.items.Items()
	out := make(map[string]T, len(items))
	for k, item := range items {
		out[castCacheValue[string](k)] = castCacheValue[T](item.Value())
	}
	return out
}

func (cache *Cache[T]) ForEach(handler func(string, T)) {
	cache.core.items.Range(func(item *ttlcache.Item[any, any]) bool {
		handler(castCacheValue[string](item.Key()), castCacheValue[T](item.Value()))
		return true
	})
}

func (cache *Cache[T]) SetExpirationCallback(callback expireCallbackWithoutReason[string, T]) {
	if callback == nil {
		cache.core.setExpirationCallback(nil)
		return
	}
	want := cache.evictReason
	cache.core.setExpirationCallback(func(key, value any, reason EvictionReason) {
		if want != 0 && reason != want {
			return
		}
		callback(castCacheValue[string](key), castCacheValue[T](value))
	})
}

func (cache *Cache[T]) SetNewItemCallback(callback itemCallback[string, T]) {
	if callback == nil {
		cache.core.setNewItemCallback(nil)
		return
	}
	cache.core.setNewItemCallback(func(key, value any) {
		callback(castCacheValue[string](key), castCacheValue[T](value))
	})
}

func (cache *Cache[T]) GetOrLoad(key string, dataLoader func() (T, error)) (T, error) {
	raw, err := cache.core.getOrLoad(key, func() (any, error) {
		return dataLoader()
	})
	return castCacheValue[T](raw), err
}

// NewTTLCache is a helper to create instance of the Cache struct
func NewTTLCache[T any](ttls ...time.Duration) *Cache[T] {
	return &Cache[T]{
		core:        newCacheExCore(WithCacheTTL(ttls...)),
		evictReason: EvictionReasonExpired,
	}
}

// NewTTLCacheWithKey is a helper to create instance of the CacheWithKey struct, allow set Key and Value
func NewTTLCacheWithKey[U comparable, T any](ttls ...time.Duration) *CacheWithKey[U, T] {
	return &CacheWithKey[U, T]{
		core:        newCacheExCore(WithCacheTTL(ttls...)),
		evictReason: EvictionReasonExpired,
	}
}

func NewLRUCache[T any](capacity uint64) *Cache[T] {
	return &Cache[T]{
		core:        newCacheExCore(WithCacheCapacity(capacity)),
		evictReason: EvictionReasonCapacityReached,
	}
}

func NewLRUCacheWithKey[U comparable, T any](capacity uint64) *CacheWithKey[U, T] {
	return &CacheWithKey[U, T]{
		core:        newCacheExCore(WithCacheCapacity(capacity)),
		evictReason: EvictionReasonCapacityReached,
	}
}
