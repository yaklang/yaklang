package utils

import (
	"context"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/samber/lo"
)

// CheckExpireCallback is used as a callback for an external check on item expiration
type checkExpireCallback[U comparable, T any] func(key U, value T) bool

// ExpireCallback is used as a callback on item expiration or when notifying of an item new to the cache
type expireCallback[U comparable, T any] func(key U, value T)

type Cache[T any] struct {
	*CacheWithKey[string, T]
}

// CacheWithKey is a synchronized map of items that can auto-expire once stale
type CacheWithKey[U comparable, T any] struct {
	*ttlcache.Cache[U, T]
	expireCallback      expireCallback[U, T]
	checkExpireCallback checkExpireCallback[U, T]
	newItemCallback     expireCallback[U, T]
	ttl                 time.Duration
	skipTTLExtension    bool
	stopOnce            *sync.Once
}

// Can only close once
func (cache *CacheWithKey[U, T]) Close() {
	cache.stopOnce.Do(func() {
		cache.Cache.DeleteAll()
		cache.Cache.Stop()
	})
}

// Set is a thread-safe way to add new items to the map
func (cache *CacheWithKey[U, T]) Set(key U, value T) {
	cache.Cache.Set(key, value, cache.ttl)
}

// SetWithTTL is a thread-safe way to add new items to the map with individual ttl
func (cache *CacheWithKey[U, T]) SetWithTTL(key U, value T, ttl time.Duration) {
	cache.Cache.Set(key, value, ttl)
}

// Get is a thread-safe way to lookup items
// Every lookup, also touches the item, hence extending it's life
func (cache *CacheWithKey[U, T]) Get(key U) (value T, exists bool) {
	var item *ttlcache.Item[U, T]
	if cache.skipTTLExtension {
		item = cache.Cache.Get(key, ttlcache.WithDisableTouchOnHit[U, T]())
	} else {
		item = cache.Cache.Get(key)
	}
	if item == nil {
		return
	}
	return item.Value(), true
}

func (cache *CacheWithKey[U, T]) GetAll() map[U]T {
	return lo.MapEntries(cache.Cache.Items(), func(key U, value *ttlcache.Item[U, T]) (U, T) {
		return key, value.Value()
	})
}

func (cache *CacheWithKey[U, T]) ForEach(handler func(U, T)) {
	cache.Cache.Range(func(item *ttlcache.Item[U, T]) bool {
		handler(item.Key(), item.Value())
		return true
	})
}

func (cache *CacheWithKey[U, T]) Remove(key U) bool {
	_, ok := cache.Cache.GetAndDelete(key)
	return ok
}

// Count returns the number of items in the cache
func (cache *CacheWithKey[U, T]) Count() int {
	return cache.Cache.Len()
}

func (cache *CacheWithKey[U, T]) SetTTL(ttl time.Duration) {
	cache.ttl = ttl
}

// SetExpirationCallback sets a callback that will be called when an item expires
func (cache *CacheWithKey[U, T]) SetExpirationCallback(callback expireCallback[U, T]) {
	// cache.OnEviction(fn func(context.Context, ttlcache.EvictionReason, *ttlcache.Item[U, T]))
	cache.expireCallback = callback
}

// SetCheckExpirationCallback sets a callback that will be called when an item is about to expire
// in order to allow external code to decide whether the item expires or remains for another TTL cycle
func (cache *CacheWithKey[U, T]) SetCheckExpirationCallback(callback checkExpireCallback[U, T]) {
	cache.checkExpireCallback = callback
}

// SetNewItemCallback sets a callback that will be called when a new item is added to the cache
func (cache *CacheWithKey[U, T]) SetNewItemCallback(callback expireCallback[U, T]) {
	cache.newItemCallback = callback
}

// SkipTtlExtensionOnHit allows the user to change the cache behaviour. When this flag is set to true it will
// no longer extend TTL of items when they are retrieved using Get, or when their expiration condition is evaluated
// using SetCheckExpirationCallback.
func (cache *CacheWithKey[U, T]) SkipTtlExtensionOnHit(value bool) {
	cache.skipTTLExtension = value
}

// Purge will remove all entries
func (cache *CacheWithKey[U, T]) Purge() {
	cache.Cache.DeleteAll()
}

// NewTTLCache is a helper to create instance of the Cache struct
func NewTTLCache[T any](ttls ...time.Duration) *Cache[T] {
	return &Cache[T]{
		CacheWithKey: NewTTLCacheWithKey[string, T](ttls...),
	}
}

// NewTTLCacheWithKey is a helper to create instance of the CacheWithKey struct, allow set Key and Value
func NewTTLCacheWithKey[U comparable, T any](ttls ...time.Duration) *CacheWithKey[U, T] {
	cache := &CacheWithKey[U, T]{
		Cache:    ttlcache.New[U, T](),
		stopOnce: new(sync.Once),
	}
	if len(ttls) > 0 {
		cache.ttl = ttls[0]
	}
	cache.Cache.OnEviction(func(ctx context.Context, reason ttlcache.EvictionReason, i *ttlcache.Item[U, T]) {
		if reason != ttlcache.EvictionReasonExpired {
			return
		}
		if cache.checkExpireCallback != nil {
			if !cache.checkExpireCallback(i.Key(), i.Value()) {
				if !cache.skipTTLExtension {
					cache.Cache.Set(i.Key(), i.Value(), i.TTL())
				}
				return
			}
		}
		if cache.expireCallback != nil {
			cache.expireCallback(i.Key(), i.Value())
		}
	})
	cache.Cache.OnInsertion(func(ctx context.Context, i *ttlcache.Item[U, T]) {
		if cache.newItemCallback != nil {
			cache.newItemCallback(i.Key(), i.Value())
		}
	})
	go cache.Cache.Start()
	return cache
}

func min(duration time.Duration, second time.Duration) time.Duration {
	if duration < second {
		return duration
	}
	return second
}
