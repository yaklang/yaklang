package utils

import (
	"container/heap"
	"sync"
	"time"
)

const (
	// ItemNotExpire Will avoid the item being expired by TTL, but can still be exired by callback etc.
	ItemNotExpire time.Duration = -1
	// ItemExpireWithGlobalTTL will use the global TTL when set.
	ItemExpireWithGlobalTTL time.Duration = 0
)

func newPriorityQueue[U comparable, T any]() *priorityQueue[U, T] {
	queue := &priorityQueue[U, T]{}
	heap.Init(queue)
	return queue
}

type priorityQueue[U comparable, T any] struct {
	items []*item[U, T]
}

func (pq *priorityQueue[U, T]) update(item *item[U, T]) {
	heap.Fix(pq, item.queueIndex)
}

func (pq *priorityQueue[U, T]) push(item *item[U, T]) {
	heap.Push(pq, item)
}

func (pq *priorityQueue[U, T]) pop() *item[U, T] {
	if pq.Len() == 0 {
		return nil
	}
	return heap.Pop(pq).(*item[U, T])
}

func (pq *priorityQueue[U, T]) remove(item *item[U, T]) {
	heap.Remove(pq, item.queueIndex)
}

func (pq priorityQueue[U, T]) Len() int {
	length := len(pq.items)
	return length
}

// Less will consider items with time.Time default value (epoch start) as more than set items.
func (pq priorityQueue[U, T]) Less(i, j int) bool {
	if pq.items[i].expireAt.IsZero() {
		return false
	}
	if pq.items[j].expireAt.IsZero() {
		return true
	}
	return pq.items[i].expireAt.Before(pq.items[j].expireAt)
}

func (pq priorityQueue[U, T]) Swap(i, j int) {
	pq.items[i], pq.items[j] = pq.items[j], pq.items[i]
	pq.items[i].queueIndex = i
	pq.items[j].queueIndex = j
}

func (pq *priorityQueue[U, T]) Push(x any) {
	item := x.(*item[U, T])
	item.queueIndex = len(pq.items)
	pq.items = append(pq.items, item)
}

func (pq *priorityQueue[U, T]) Pop() any {
	old := pq.items
	n := len(old)
	item := old[n-1]
	item.queueIndex = -1
	pq.items = old[0 : n-1]
	return item
}

func newItem[U comparable, T any](key U, data T, ttl time.Duration) *item[U, T] {
	item := &item[U, T]{
		data: data,
		ttl:  ttl,
		key:  key,
	}
	// since nobody is aware yet of this item, it's safe to touch without lock here
	item.touch()
	return item
}

type item[U comparable, T any] struct {
	key        U
	data       T
	ttl        time.Duration
	expireAt   time.Time
	queueIndex int
}

// Reset the item expiration time
func (item *item[U, T]) touch() {
	if item.ttl > 0 {
		item.expireAt = time.Now().Add(item.ttl)
	}
}

// Verify if the item is expired
func (item *item[U, T]) expired() bool {
	if item.ttl <= 0 {
		return false
	}
	return item.expireAt.Before(time.Now())
}

// CheckExpireCallback is used as a callback for an external check on item expiration
type checkExpireCallback[U comparable, T any] func(key U, value T) bool

// ExpireCallback is used as a callback on item expiration or when notifying of an item new to the cache
type expireCallback[U comparable, T any] func(key U, value T)

// Cache is a synchronized map of items that can auto-expire once stale
type Cache[T any] struct {
	*CacheWithKey[string, T]
}

type CacheWithKey[U comparable, T any] struct {
	mutex                  sync.Mutex
	ttl                    time.Duration
	items                  map[U]*item[U, T]
	expireCallback         expireCallback[U, T]
	checkExpireCallback    checkExpireCallback[U, T]
	newItemCallback        expireCallback[U, T]
	priorityQueue          *priorityQueue[U, T]
	expirationNotification chan bool
	expirationTime         time.Time
	skipTTLExtension       bool
	shutdownSignal         chan (chan struct{})
	isShutDown             bool
}

func (cache *CacheWithKey[U, T]) getItem(key U) (*item[U, T], bool, bool) {
	item, exists := cache.items[key]
	if !exists || item.expired() {
		return nil, false, false
	}

	if item.ttl >= 0 && (item.ttl > 0 || cache.ttl > 0) {
		if cache.ttl > 0 && item.ttl == 0 {
			item.ttl = cache.ttl
		}

		if !cache.skipTTLExtension {
			item.touch()
		}
		cache.priorityQueue.update(item)
	}

	expirationNotification := false
	if cache.expirationTime.After(time.Now().Add(item.ttl)) {
		expirationNotification = true
	}
	return item, exists, expirationNotification
}

func (cache *CacheWithKey[U, T]) startExpirationProcessing() {
	timer := time.NewTimer(time.Hour)
	for {
		var sleepTime time.Duration
		cache.mutex.Lock()
		if cache.priorityQueue.Len() > 0 {
			sleepTime = time.Until(cache.priorityQueue.items[0].expireAt)
			if sleepTime < 0 && cache.priorityQueue.items[0].expireAt.IsZero() {
				sleepTime = time.Hour
			} else if sleepTime < 0 {
				sleepTime = time.Microsecond
			}
			if cache.ttl > 0 {
				sleepTime = min(sleepTime, cache.ttl)
			}

		} else if cache.ttl > 0 {
			sleepTime = cache.ttl
		} else {
			sleepTime = time.Hour
		}

		cache.expirationTime = time.Now().Add(sleepTime)
		cache.mutex.Unlock()

		timer.Reset(sleepTime)
		select {
		case shutdownFeedback := <-cache.shutdownSignal:
			timer.Stop()
			shutdownFeedback <- struct{}{}
			return
		case <-timer.C:
			timer.Stop()
			cache.mutex.Lock()
			if cache.priorityQueue.Len() == 0 {
				cache.mutex.Unlock()
				continue
			}

			// index will only be advanced if the current entry will not be evicted
			i := 0
			for item := cache.priorityQueue.items[i]; item.expired(); item = cache.priorityQueue.items[i] {

				if cache.checkExpireCallback != nil {
					if !cache.checkExpireCallback(item.key, item.data) {
						item.touch()
						cache.priorityQueue.update(item)
						i++
						if i == cache.priorityQueue.Len() {
							break
						}
						continue
					}
				}

				cache.priorityQueue.remove(item)
				delete(cache.items, item.key)
				if cache.expireCallback != nil {
					go cache.expireCallback(item.key, item.data)
				}
				if cache.priorityQueue.Len() == 0 {
					goto done
				}
			}
		done:
			cache.mutex.Unlock()

		case <-cache.expirationNotification:
			timer.Stop()
			continue
		}
	}
}

// Close calls Purge, and then stops the goroutine that does ttl checking, for a clean shutdown.
// The cache is no longer cleaning up after the first call to Close, repeated calls are safe though.
func (cache *CacheWithKey[U, T]) Close() {
	cache.mutex.Lock()
	if !cache.isShutDown {
		cache.isShutDown = true
		cache.mutex.Unlock()
		feedback := make(chan struct{})
		cache.shutdownSignal <- feedback
		<-feedback
		close(cache.shutdownSignal)
	} else {
		cache.mutex.Unlock()
	}
	cache.Purge()
}

// Set is a thread-safe way to add new items to the map
func (cache *CacheWithKey[U, T]) Set(key U, data T) {
	cache.SetWithTTL(key, data, ItemExpireWithGlobalTTL)
}

// SetWithTTL is a thread-safe way to add new items to the map with individual ttl
func (cache *CacheWithKey[U, T]) SetWithTTL(key U, data T, ttl time.Duration) {
	cache.mutex.Lock()
	item, exists, _ := cache.getItem(key)

	if exists {
		item.data = data
		item.ttl = ttl
	} else {
		item = newItem(key, data, ttl)
		cache.items[key] = item
	}

	if item.ttl >= 0 && (item.ttl > 0 || cache.ttl > 0) {
		if cache.ttl > 0 && item.ttl == 0 {
			item.ttl = cache.ttl
		}
		item.touch()
	}

	if exists {
		cache.priorityQueue.update(item)
	} else {
		cache.priorityQueue.push(item)
	}

	cache.mutex.Unlock()
	if !exists && cache.newItemCallback != nil {
		cache.newItemCallback(key, data)
	}
	cache.expirationNotification <- true
}

// Get is a thread-safe way to lookup items
// Every lookup, also touches the item, hence extending it's life
func (cache *CacheWithKey[U, T]) Get(key U) (value T, exists bool) {
	cache.mutex.Lock()
	item, exists, triggerExpirationNotification := cache.getItem(key)
	cache.mutex.Unlock()
	if triggerExpirationNotification {
		cache.expirationNotification <- true
	}
	if !exists {
		return
	}
	return item.data, exists
}

func (cache *CacheWithKey[U, T]) GetAll() []T {
	cache.mutex.Lock()
	items := make([]T, 0, len(cache.items))
	for key := range cache.items {
		item, exists, triggerExpirationNotification := cache.getItem(key)
		if triggerExpirationNotification {
			cache.expirationNotification <- true
		}
		if exists {
			items = append(items, item.data)
		}
	}
	cache.mutex.Unlock()
	return items
}

func (cache *CacheWithKey[U, T]) Remove(key U) bool {
	cache.mutex.Lock()
	object, exists := cache.items[key]
	if !exists {
		cache.mutex.Unlock()
		return false
	}
	delete(cache.items, object.key)
	cache.priorityQueue.remove(object)
	cache.mutex.Unlock()

	return true
}

// Count returns the number of items in the cache
func (cache *CacheWithKey[U, T]) Count() int {
	cache.mutex.Lock()
	length := len(cache.items)
	cache.mutex.Unlock()
	return length
}

func (cache *CacheWithKey[U, T]) SetTTL(ttl time.Duration) {
	cache.mutex.Lock()
	cache.ttl = ttl
	cache.mutex.Unlock()
	cache.expirationNotification <- true
}

// SetExpirationCallback sets a callback that will be called when an item expires
func (cache *CacheWithKey[U, T]) SetExpirationCallback(callback expireCallback[U, T]) {
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
	cache.mutex.Lock()
	cache.items = make(map[U]*item[U, T])
	cache.priorityQueue = newPriorityQueue[U, T]()
	cache.mutex.Unlock()
}

// NewTTLCache is a helper to create instance of the Cache struct
func NewTTLCache[T any](ttls ...time.Duration) *Cache[T] {
	return &Cache[T]{
		CacheWithKey: NewTTLCacheWithKey[string, T](ttls...),
	}

}

// NewTTLCacheWithKey is a helper to create instance of the CacheWithKey struct, allow set Key and Value
func NewTTLCacheWithKey[U comparable, T any](ttls ...time.Duration) *CacheWithKey[U, T] {
	shutdownChan := make(chan chan struct{})

	cache := &CacheWithKey[U, T]{
		items:                  make(map[U]*item[U, T]),
		priorityQueue:          newPriorityQueue[U, T](),
		expirationNotification: make(chan bool),
		expirationTime:         time.Now(),
		shutdownSignal:         shutdownChan,
		isShutDown:             false,
	}
	if len(ttls) > 0 {
		cache.ttl = ttls[0]
	}
	go cache.startExpirationProcessing()
	return cache
}

func min(duration time.Duration, second time.Duration) time.Duration {
	if duration < second {
		return duration
	}
	return second
}
