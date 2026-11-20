package utils

import (
	"context"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
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

// cacheExCore holds the non-generic implementation shared by every
// CacheExWithKey[U, T] instantiation. Keeping ttlcache.Cache[any, any] and the
// single-flight state machine non-generic prevents hundreds of generic
// instantiations and promoted ttlcache method wrappers from being retained by
// dynamic reflection (reflect.MethodByName).
type cacheExCore struct {
	items                 *ttlcache.Cache[any, any]
	ctx                   context.Context
	cancel                context.CancelFunc
	config                *cacheExConfig
	evictionCallbackClear func()
	stopOnce              *sync.Once

	// configMu protects the mutable configuration fields below. These fields
	// are read by the background insertion/eviction callback goroutines (started
	// inside reset) and by Get/Set, while being written by the Set* methods, so
	// they must be synchronized to avoid data races and heap corruption.
	configMu         sync.RWMutex
	expireCallback   func(key, value any, reason EvictionReason)
	newItemCallback  func(key, value any)
	ttl              time.Duration
	skipTTLExtension bool

	// Single-flight functionality
	flightEntries map[any]*cacheFlightEntry
	flightMu      sync.RWMutex // Protects access to the 'flightEntries' map
}

type cacheFlightEntry struct {
	data      any
	err       error
	preparing bool
	cond      *sync.Cond
	createdAt time.Time
}

func newCacheExCore(opt ...cacheExOption) *cacheExCore {
	ctx, cancel := context.WithCancel(context.Background())
	config := &cacheExConfig{}
	for _, o := range opt {
		o(config)
	}
	core := &cacheExCore{
		ctx:      ctx,
		cancel:   cancel,
		config:   config,
		ttl:      config.ttl,
		stopOnce: &sync.Once{},
	}
	core.reset()
	return core
}

func (c *cacheExCore) getTTL() time.Duration {
	c.configMu.RLock()
	ttl := c.ttl
	c.configMu.RUnlock()
	return ttl
}

func (c *cacheExCore) close() {
	if c.stopOnce == nil {
		c.stopOnce = &sync.Once{}
	}
	c.stopOnce.Do(func() {
		c.cancel()
		c.items.DeleteAll()
		c.items.Stop()
		c.evictionCallbackClear()

		c.flightMu.Lock()
		c.flightEntries = make(map[any]*cacheFlightEntry)
		c.flightMu.Unlock()
	})
}

func (c *cacheExCore) set(key, value any) {
	c.items.Set(key, value, c.getTTL())
}

func (c *cacheExCore) setWithTTL(key, value any, ttl time.Duration) {
	c.items.Set(key, value, ttl)
}

func (c *cacheExCore) get(key any) (any, bool) {
	c.configMu.RLock()
	skipTTLExtension := c.skipTTLExtension
	c.configMu.RUnlock()
	var item *ttlcache.Item[any, any]
	if skipTTLExtension {
		item = c.items.Get(key, ttlcache.WithDisableTouchOnHit[any, any]())
	} else {
		item = c.items.Get(key)
	}
	if item == nil {
		return nil, false
	}
	return item.Value(), true
}

func (c *cacheExCore) remove(key any) bool {
	_, ok := c.items.GetAndDelete(key)
	return ok
}

func (c *cacheExCore) deleteKey(key any) {
	c.items.Delete(key)
}

func (c *cacheExCore) deleteAll() {
	c.items.DeleteAll()
}

func (c *cacheExCore) count() int {
	return c.items.Len()
}

func (c *cacheExCore) setTTL(ttl time.Duration) {
	c.configMu.Lock()
	c.ttl = ttl
	c.configMu.Unlock()
}

func (c *cacheExCore) setExpirationCallback(cb func(key, value any, reason EvictionReason)) {
	c.configMu.Lock()
	c.expireCallback = cb
	c.configMu.Unlock()
}

func (c *cacheExCore) setNewItemCallback(cb func(key, value any)) {
	c.configMu.Lock()
	c.newItemCallback = cb
	c.configMu.Unlock()
}

func (c *cacheExCore) skipTtlExtensionOnHit(value bool) {
	c.configMu.Lock()
	c.skipTTLExtension = value
	c.configMu.Unlock()
}

func (c *cacheExCore) purge() {
	c.items.DeleteAll()

	c.flightMu.Lock()
	c.flightEntries = make(map[any]*cacheFlightEntry)
	c.flightMu.Unlock()
}

func (c *cacheExCore) getOrLoad(key any, dataLoader func() (any, error)) (any, error) {
	if item := c.items.Get(key); item != nil {
		return item.Value(), nil
	}

	c.flightMu.RLock()
	entry, found := c.flightEntries[key]
	c.flightMu.RUnlock()

	if found {
		entry.cond.L.Lock()
		for entry.preparing {
			entry.cond.Wait()
		}
		data, err := entry.data, entry.err
		entry.cond.L.Unlock()

		if err == nil {
			c.items.Set(key, data, c.getTTL())
		}
		return data, err
	}

	c.flightMu.Lock()
	entry, found = c.flightEntries[key]
	if !found {
		entry = &cacheFlightEntry{
			preparing: true,
			cond:      sync.NewCond(&sync.Mutex{}),
			createdAt: time.Now(),
		}
		c.flightEntries[key] = entry
	}
	c.flightMu.Unlock()

	entry.cond.L.Lock()
	if !entry.preparing {
		data, err := entry.data, entry.err
		entry.cond.L.Unlock()

		if err == nil {
			c.items.Set(key, data, c.getTTL())
		}
		return data, err
	}

	data, err := dataLoader()

	entry.data = data
	entry.err = err
	entry.preparing = false
	entry.cond.Broadcast()
	entry.cond.L.Unlock()

	if err == nil {
		c.items.Set(key, data, c.getTTL())
	}

	c.flightMu.Lock()
	delete(c.flightEntries, key)
	c.flightMu.Unlock()

	return data, err
}

func (c *cacheExCore) reset() {
	c.items = ttlcache.New[any, any](
		ttlcache.WithCapacity[any, any](c.config.capacity),
	)
	if c.stopOnce == nil {
		c.stopOnce = &sync.Once{}
	}

	c.flightEntries = make(map[any]*cacheFlightEntry)

	c.evictionCallbackClear = c.items.OnEviction(func(ctx context.Context, rawReason ttlcache.EvictionReason, i *ttlcache.Item[any, any]) {
		reason := EvictionReason(rawReason)
		c.configMu.RLock()
		expireCallback := c.expireCallback
		c.configMu.RUnlock()
		if expireCallback != nil {
			expireCallback(i.Key(), i.Value(), reason)
		}

		c.flightMu.Lock()
		delete(c.flightEntries, i.Key())
		c.flightMu.Unlock()
	})
	c.items.OnInsertion(func(ctx context.Context, i *ttlcache.Item[any, any]) {
		c.configMu.RLock()
		newItemCallback := c.newItemCallback
		c.configMu.RUnlock()
		if newItemCallback != nil {
			newItemCallback(i.Key(), i.Value())
		}
	})

	go c.cleanupFlightEntries()
	go c.items.Start()
}

func (c *cacheExCore) cleanupFlightEntries() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.flightMu.Lock()
			cutoff := time.Now().Add(-5 * time.Minute)
			for key, entry := range c.flightEntries {
				entry.cond.L.Lock()
				preparing := entry.preparing
				entry.cond.L.Unlock()
				if !preparing && entry.createdAt.Before(cutoff) {
					delete(c.flightEntries, key)
				}
			}
			c.flightMu.Unlock()
		}
	}
}

func castCacheValue[T any](v any) T {
	if v == nil {
		var zero T
		return zero
	}
	return v.(T)
}

type CacheEx[T any] struct {
	*CacheExWithKey[string, T]
}

// CacheExWithKey is a synchronized map of items that can auto-expire once stale
type CacheExWithKey[U comparable, T any] struct {
	core *cacheExCore
}

// Close stops the cache's background workers. It is safe to call more than
// once: ttlcache.Stop sends on an unbuffered channel that nothing reads after
// the first stop, so a second call would block the caller forever. Callers
// reach this from both an explicit close and the owning parent's eviction
// callback, so idempotency is required, not optional.
func (cache *CacheExWithKey[U, T]) Close() {
	cache.core.close()
}

// Set is a thread-safe way to add new items to the map
func (cache *CacheExWithKey[U, T]) Set(key U, value T) {
	cache.core.set(key, value)
}

// SetWithTTL is a thread-safe way to add new items to the map with individual ttl
func (cache *CacheExWithKey[U, T]) SetWithTTL(key U, value T, ttl time.Duration) {
	cache.core.setWithTTL(key, value, ttl)
}

// Get is a thread-safe way to lookup items
// Every lookup, also touches the item, hence extending it's life
func (cache *CacheExWithKey[U, T]) Get(key U) (value T, exists bool) {
	raw, ok := cache.core.get(key)
	if !ok {
		return value, false
	}
	return castCacheValue[T](raw), true
}

func (cache *CacheExWithKey[U, T]) GetAll() map[U]T {
	items := cache.core.items.Items()
	out := make(map[U]T, len(items))
	for k, item := range items {
		out[castCacheValue[U](k)] = castCacheValue[T](item.Value())
	}
	return out
}

func (cache *CacheExWithKey[U, T]) ForEach(handler func(U, T)) {
	cache.core.items.Range(func(item *ttlcache.Item[any, any]) bool {
		handler(castCacheValue[U](item.Key()), castCacheValue[T](item.Value()))
		return true
	})
}

func (cache *CacheExWithKey[U, T]) Remove(key U) bool {
	return cache.core.remove(key)
}

// Delete removes key without touching or returning the item.
func (cache *CacheExWithKey[U, T]) Delete(key U) {
	cache.core.deleteKey(key)
}

// DeleteAll removes all items from the underlying cache.
func (cache *CacheExWithKey[U, T]) DeleteAll() {
	cache.core.deleteAll()
}

// Count returns the number of items in the cache
func (cache *CacheExWithKey[U, T]) Count() int {
	return cache.core.count()
}

func (cache *CacheExWithKey[U, T]) SetTTL(ttl time.Duration) {
	cache.core.setTTL(ttl)
}

// SetExpirationCallback sets a callback that will be called when an item expires
func (cache *CacheExWithKey[U, T]) SetExpirationCallback(callback expireCallback[U, T]) {
	if callback == nil {
		cache.core.setExpirationCallback(nil)
		return
	}
	cache.core.setExpirationCallback(func(key, value any, reason EvictionReason) {
		callback(castCacheValue[U](key), castCacheValue[T](value), reason)
	})
}

// SetNewItemCallback sets a callback that will be called when a new item is added to the cache
func (cache *CacheExWithKey[U, T]) SetNewItemCallback(callback itemCallback[U, T]) {
	if callback == nil {
		cache.core.setNewItemCallback(nil)
		return
	}
	cache.core.setNewItemCallback(func(key, value any) {
		callback(castCacheValue[U](key), castCacheValue[T](value))
	})
}

// SkipTtlExtensionOnHit allows the user to change the cache behaviour. When this flag is set to true it will
// no longer extend TTL of items when they are retrieved using Get, or when their expiration condition is evaluated
// using SetCheckExpirationCallback.
func (cache *CacheExWithKey[U, T]) SkipTtlExtensionOnHit(value bool) {
	cache.core.skipTtlExtensionOnHit(value)
}

// Purge will remove all entries
func (cache *CacheExWithKey[U, T]) Purge() {
	cache.core.purge()
}

// GetOrLoad attempts to retrieve data from the cache for the given key.
// If the data is not present, or is currently being prepared by another goroutine,
// it will wait for the data to become ready. If no preparation is in progress,
// it initiates the data preparation using the provided dataLoader function.
//
// This method provides single-flight behavior: for a given key, the dataLoader function
// is executed only once concurrently. Multiple concurrent requests for the same key
// will wait for the single loading operation to complete and then receive its result.
//
// The dataLoader function should be idempotent and thread-safe if it's external,
// as it will be executed by only one goroutine for a given key at a time.
func (c *CacheExWithKey[U, T]) GetOrLoad(key U, dataLoader func() (T, error)) (T, error) {
	raw, err := c.core.getOrLoad(key, func() (any, error) {
		return dataLoader()
	})
	return castCacheValue[T](raw), err
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
	return &CacheExWithKey[U, T]{
		core: newCacheExCore(opt...),
	}
}
