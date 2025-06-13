package utils

import (
	"context"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/samber/lo"
)

// Available eviction reasons.
const (
	EvictionReasonDeleted EvictionReason = iota + 1
	EvictionReasonCapacityReached
	EvictionReasonExpired
)

// EvictionReason is used to specify why a certain item was
// evicted/deleted.
type EvictionReason int

// ExpireCallback is used as a callback on item expiration or when notifying of an item new to the cache
type expireCallback[U comparable, T any] func(key U, value T, reason EvictionReason)

// NewItemCallback is used as a callback on item expiration or when notifying of an item new to the cache
type itemCallback[U comparable, T any] func(key U, value T)

type CacheEx[T any] struct {
	*CacheExWithKey[string, T]
}

// CacheExWithKey is a synchronized map of items that can auto-expire once stale
type CacheExWithKey[U comparable, T any] struct {
	*ttlcache.Cache[U, T]
	config                *cacheExConfig
	expireCallback        expireCallback[U, T]
	newItemCallback       itemCallback[U, T]
	ttl                   time.Duration
	skipTTLExtension      bool
	evictionCallbackClear func()
	stopOnce              *sync.Once
}

// Can only close once
func (cache *CacheExWithKey[U, T]) Close() {
	// close
	cache.Cache.DeleteAll()
	cache.Cache.Stop()
	cache.evictionCallbackClear()

	// reset
	cache.reset()
}

// Set is a thread-safe way to add new items to the map
func (cache *CacheExWithKey[U, T]) Set(key U, value T) {
	cache.Cache.Set(key, value, cache.ttl)
}

// SetWithTTL is a thread-safe way to add new items to the map with individual ttl
func (cache *CacheExWithKey[U, T]) SetWithTTL(key U, value T, ttl time.Duration) {
	cache.Cache.Set(key, value, ttl)
}

// Get is a thread-safe way to lookup items
// Every lookup, also touches the item, hence extending it's life
func (cache *CacheExWithKey[U, T]) Get(key U) (value T, exists bool) {
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

func (cache *CacheExWithKey[U, T]) GetAll() map[U]T {
	return lo.MapEntries(cache.Cache.Items(), func(key U, value *ttlcache.Item[U, T]) (U, T) {
		return key, value.Value()
	})
}

func (cache *CacheExWithKey[U, T]) ForEach(handler func(U, T)) {
	cache.Cache.Range(func(item *ttlcache.Item[U, T]) bool {
		handler(item.Key(), item.Value())
		return true
	})
}

func (cache *CacheExWithKey[U, T]) Remove(key U) bool {
	_, ok := cache.Cache.GetAndDelete(key)
	return ok
}

// Count returns the number of items in the cache
func (cache *CacheExWithKey[U, T]) Count() int {
	return cache.Cache.Len()
}

func (cache *CacheExWithKey[U, T]) SetTTL(ttl time.Duration) {
	cache.ttl = ttl
}

// SetExpirationCallback sets a callback that will be called when an item expires
func (cache *CacheExWithKey[U, T]) SetExpirationCallback(callback expireCallback[U, T]) {
	// cache.OnEviction(fn func(context.Context, ttlcache.EvictionReason, *ttlcache.Item[U, T]))
	cache.expireCallback = callback
}

// SetNewItemCallback sets a callback that will be called when a new item is added to the cache
func (cache *CacheExWithKey[U, T]) SetNewItemCallback(callback itemCallback[U, T]) {
	cache.newItemCallback = callback
}

// SkipTtlExtensionOnHit allows the user to change the cache behaviour. When this flag is set to true it will
// no longer extend TTL of items when they are retrieved using Get, or when their expiration condition is evaluated
// using SetCheckExpirationCallback.
func (cache *CacheExWithKey[U, T]) SkipTtlExtensionOnHit(value bool) {
	cache.skipTTLExtension = value
}

// Purge will remove all entries
func (cache *CacheExWithKey[U, T]) Purge() {
	cache.Cache.DeleteAll()
}

func min(duration time.Duration, second time.Duration) time.Duration {
	if duration < second {
		return duration
	}
	return second
}

type cacheExConfig struct {
	capacity uint64
	ttl      time.Duration
}
type cacheExOption func(*cacheExConfig)

func WithCacheCapacity(capacity uint64) cacheExOption {
	return func(c *cacheExConfig) {
		c.capacity = capacity
	}
}

func WithCacheTTL(ttl ...time.Duration) cacheExOption {
	return func(c *cacheExConfig) {
		if len(ttl) > 0 {
			c.ttl = ttl[0]
		}
	}
}

func NewCacheEx[T any](opt ...cacheExOption) *CacheEx[T] {
	return &CacheEx[T]{
		CacheExWithKey: NewCacheExWithKey[string, T](opt...),
	}
}

func NewCacheExWithKey[U comparable, T any](opt ...cacheExOption) *CacheExWithKey[U, T] {
	config := &cacheExConfig{}
	for _, o := range opt {
		o(config)
	}

	cache := &CacheExWithKey[U, T]{
		config: config,
		ttl:    config.ttl,
	}
	cache.reset()
	return cache
}

func (c *CacheExWithKey[U, T]) reset() {
	c.Cache = ttlcache.New[U, T](
		ttlcache.WithCapacity[U, T](c.config.capacity),
	)

	c.evictionCallbackClear = c.Cache.OnEviction(func(ctx context.Context, raw_reason ttlcache.EvictionReason, i *ttlcache.Item[U, T]) {
		reason := EvictionReason(raw_reason)
		if c.expireCallback != nil {
			c.expireCallback(i.Key(), i.Value(), reason)
		}
	})
	c.Cache.OnInsertion(func(ctx context.Context, i *ttlcache.Item[U, T]) {
		if c.newItemCallback != nil {
			c.newItemCallback(i.Key(), i.Value())
		}
	})
	go c.Cache.Start()
}
