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
	ctx                   context.Context
	cancel                context.CancelFunc
	config                *cacheExConfig
	expireCallback        expireCallback[U, T]
	newItemCallback       itemCallback[U, T]
	ttl                   time.Duration
	skipTTLExtension      bool
	evictionCallbackClear func()
	stopOnce              *sync.Once

	// Single-flight functionality
	flightEntries map[U]*flightEntry[T]
	flightMu      sync.RWMutex // Protects access to the 'flightEntries' map
}

// flightEntry represents a single-flight loading operation
type flightEntry[T any] struct {
	data      T          // The actual data stored
	err       error      // Error if data loading failed
	preparing bool       // True if data is currently being prepared by one goroutine
	cond      *sync.Cond // Condition variable to signal when data is ready
	createdAt time.Time  // Time when the entry was created
}

// Can only close once
func (cache *CacheExWithKey[U, T]) Close() {
	cache.cancel()
	// close
	cache.Cache.DeleteAll()
	cache.Cache.Stop()
	cache.evictionCallbackClear()

	// Clean up flight entries
	cache.flightMu.Lock()
	cache.flightEntries = make(map[U]*flightEntry[T])
	cache.flightMu.Unlock()
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

	// Also clear flight entries
	cache.flightMu.Lock()
	cache.flightEntries = make(map[U]*flightEntry[T])
	cache.flightMu.Unlock()
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
	// First, try to get from the main cache
	if item := c.Cache.Get(key); item != nil {
		return item.Value(), nil
	}

	// Data not in cache, need to check single-flight mechanism
	c.flightMu.RLock()
	entry, found := c.flightEntries[key]
	c.flightMu.RUnlock()

	if found {
		// An entry for this key exists in flight.
		// Acquire the entry's specific mutex to check its state and potentially wait.
		entry.cond.L.Lock()
		// If data is being prepared, wait for it to finish.
		// The loop handles spurious wakeups.
		for entry.preparing {
			entry.cond.Wait()
		}
		// Data is now ready (or preparation failed).
		data, err := entry.data, entry.err
		entry.cond.L.Unlock()

		// If successful, store in main cache
		if err == nil {
			c.Cache.Set(key, data, c.ttl)
		}

		return data, err
	}

	// Data not found in flight cache, need to create or get an entry for preparation.
	c.flightMu.Lock() // Acquire a write lock for the flight map
	// Double-check after acquiring the write lock, in case another goroutine
	// just created or finished preparing the entry.
	entry, found = c.flightEntries[key]
	if !found {
		// This goroutine is the first to request this key.
		// Create a new entry and mark it as preparing.
		entry = &flightEntry[T]{
			preparing: true,
			cond:      sync.NewCond(&sync.Mutex{}),
			createdAt: time.Now(),
		}
		c.flightEntries[key] = entry
	}
	c.flightMu.Unlock() // Release the write lock for the flight map

	// Now we have the entry (either newly created or found).
	// Acquire the entry's specific mutex to manage its preparation state.
	entry.cond.L.Lock()
	if !entry.preparing {
		// Another goroutine (that won the race to acquire the global write lock
		// before us) already finished preparing this entry.
		// We just return its result.
		data, err := entry.data, entry.err
		entry.cond.L.Unlock()

		// If successful, store in main cache
		if err == nil {
			c.Cache.Set(key, data, c.ttl)
		}

		return data, err
	}

	// If we reach here, it means this goroutine is responsible for
	// executing the dataLoader function.
	data, err := dataLoader() // Execute the actual data loading

	// Store the result and signal all waiting goroutines.
	entry.data = data
	entry.err = err
	entry.preparing = false // Mark as no longer preparing
	entry.cond.Broadcast()  // Signal all goroutines waiting on this entry
	entry.cond.L.Unlock()   // Release the mutex associated with the condition variable

	// If successful, store in main cache
	if err == nil {
		c.Cache.Set(key, data, c.ttl)
	}

	// Always clean up the flight entry after processing
	// For errors, we want subsequent calls to retry, so we don't keep the flight entry
	c.flightMu.Lock()
	delete(c.flightEntries, key)
	c.flightMu.Unlock()

	return data, err
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
	ctx, cancel := context.WithCancel(context.Background())
	config := &cacheExConfig{}
	for _, o := range opt {
		o(config)
	}

	cache := &CacheExWithKey[U, T]{
		ctx:    ctx,
		cancel: cancel,
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

	// Initialize single-flight fields
	c.flightEntries = make(map[U]*flightEntry[T])

	c.evictionCallbackClear = c.Cache.OnEviction(func(ctx context.Context, raw_reason ttlcache.EvictionReason, i *ttlcache.Item[U, T]) {
		reason := EvictionReason(raw_reason)
		if c.expireCallback != nil {
			c.expireCallback(i.Key(), i.Value(), reason)
		}

		// Clean up corresponding flight entry if it exists
		c.flightMu.Lock()
		delete(c.flightEntries, i.Key())
		c.flightMu.Unlock()
	})
	c.Cache.OnInsertion(func(ctx context.Context, i *ttlcache.Item[U, T]) {
		if c.newItemCallback != nil {
			c.newItemCallback(i.Key(), i.Value())
		}
	})

	// Start background cleanup for flight entries
	go c.cleanupFlightEntries()

	go c.Cache.Start()
}

// cleanupFlightEntries periodically cleans up old flight entries
func (c *CacheExWithKey[U, T]) cleanupFlightEntries() {
	ticker := time.NewTicker(1 * time.Minute) // Clean up every minute
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.flightMu.Lock()
			cutoff := time.Now().Add(-5 * time.Minute) // Remove entries older than 5 minutes
			for key, entry := range c.flightEntries {
				if !entry.preparing && entry.createdAt.Before(cutoff) {
					delete(c.flightEntries, key)
				}
			}
			c.flightMu.Unlock()
		}
	}
}
