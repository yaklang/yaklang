package dbcache

import (
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

type EnqueuePersist[K comparable] func(K, uint64, utils.EvictionReason) bool
type LoadFuncWithKey[K comparable, T any] func(K) (T, error)

type residentItem[K comparable, T any] struct {
	key                    K
	memoryItem             T
	generation             uint64
	pending                bool
	persisting             bool
	updatedWhilePersisting bool
	persistSnapshot        T
	persistSnapshotSet     bool
	reason                 utils.EvictionReason
}

type PersistRequest[K comparable] struct {
	Key        K
	Generation uint64
	Reason     utils.EvictionReason
}

// ResidencyCacheWithKey keeps the live in-memory copy of items and coordinates
// eviction with the async persistence pipeline. A queued item remains resident
// until FinishPersist or RejectPersist settles the matching generation.
type ResidencyCacheWithKey[K comparable, T any] struct {
	mu sync.RWMutex

	evictionCache *utils.CacheExWithKey[K, struct{}]
	data          map[K]*residentItem[K, T]

	enqueuePersist EnqueuePersist[K]
	load           LoadFuncWithKey[K, T]
	skipEviction   func(T) bool

	persistWG    sync.WaitGroup
	pendingCount atomic.Int64
	closed       atomic.Bool
	saveDisabled atomic.Bool
}

func NewResidencyCacheWithKey[K comparable, T any](
	ttl time.Duration,
	maxEntries int,
	enqueuePersist EnqueuePersist[K],
	load LoadFuncWithKey[K, T],
	skipEviction ...func(T) bool,
) *ResidencyCacheWithKey[K, T] {
	var evictSkipper func(T) bool
	if len(skipEviction) > 0 {
		evictSkipper = skipEviction[0]
	}
	ret := &ResidencyCacheWithKey[K, T]{
		data:           make(map[K]*residentItem[K, T]),
		enqueuePersist: enqueuePersist,
		load:           load,
		skipEviction:   evictSkipper,
	}

	if ttl <= 0 && maxEntries <= 0 {
		return ret
	}

	cache := utils.NewCacheExWithTTLAndCapacity[K, struct{}](ttl, maxEntries)
	cache.SetExpirationCallback(func(key K, _ struct{}, reason utils.EvictionReason) {
		ret.handleEviction(key, reason)
	})
	ret.evictionCache = cache
	return ret
}

func (c *ResidencyCacheWithKey[K, T]) Set(key K, memValue T) {
	if c == nil {
		return
	}
	closed := c.closed.Load()

	c.mu.Lock()
	if item, ok := c.data[key]; ok {
		item.memoryItem = memValue
		item.pending = false
		item.persisting = false
		item.updatedWhilePersisting = false
		item.persistSnapshot = *new(T)
		item.persistSnapshotSet = false
		item.generation++
	} else {
		c.data[key] = &residentItem[K, T]{
			key:        key,
			memoryItem: memValue,
		}
	}
	c.mu.Unlock()

	if closed || c.evictionCache == nil || c.IsSaveDisabled() {
		return
	}

	if !c.shouldSkipEviction(memValue) {
		c.evictionCache.Set(key, struct{}{})
	} else {
		c.evictionCache.Delete(key)
	}
}

func (c *ResidencyCacheWithKey[K, T]) GetResident(key K) (T, bool) {
	if c == nil {
		var zero T
		return zero, false
	}

	var (
		value T
		ok    bool
	)
	c.mu.Lock()
	if item, exists := c.data[key]; exists {
		if item.pending && !item.persisting {
			item.pending = false
			item.updatedWhilePersisting = false
			item.persistSnapshot = *new(T)
			item.persistSnapshotSet = false
		}
		value = item.memoryItem
		ok = true
	}
	c.mu.Unlock()
	if !ok {
		var zero T
		return zero, false
	}

	if c.evictionCache != nil && !c.IsSaveDisabled() && !c.shouldSkipEviction(value) {
		if _, tracked := c.evictionCache.Get(key); !tracked {
			c.evictionCache.Set(key, struct{}{})
		}
	}
	return value, true
}

func (c *ResidencyCacheWithKey[K, T]) Get(key K) (T, bool) {
	if ret, ok := c.GetResident(key); ok {
		return ret, true
	}
	if c == nil || c.load == nil {
		var zero T
		return zero, false
	}

	memValue, err := c.load(key)
	if err != nil {
		var zero T
		return zero, false
	}

	c.mu.RLock()
	if item, ok := c.data[key]; ok {
		value := item.memoryItem
		c.mu.RUnlock()
		if c.evictionCache != nil && !c.shouldSkipEviction(value) {
			c.evictionCache.Get(key)
		}
		return value, true
	}
	c.mu.RUnlock()

	c.Set(key, memValue)
	return memValue, true
}

func (c *ResidencyCacheWithKey[K, T]) SnapshotForPersist(key K, generation uint64) (T, bool) {
	if c == nil {
		var zero T
		return zero, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.data[key]
	if !ok || !item.pending || item.generation != generation {
		var zero T
		return zero, false
	}
	item.persisting = true
	item.persistSnapshot = item.memoryItem
	item.persistSnapshotSet = true
	return item.memoryItem, true
}

func (c *ResidencyCacheWithKey[K, T]) FinishPersist(key K, generation uint64, success bool) {
	if c == nil {
		return
	}

	shouldTouch := false
	var retry *PersistRequest[K]

	c.mu.Lock()
	if item, ok := c.data[key]; ok {
		if item.pending && item.generation == generation {
			item.persisting = false
			if success && item.updatedWhilePersisting {
				item.updatedWhilePersisting = false
				item.persistSnapshot = *new(T)
				item.persistSnapshotSet = false
				item.generation++
				retry = &PersistRequest[K]{
					Key:        key,
					Generation: item.generation,
					Reason:     item.reason,
				}
				c.persistWG.Add(1)
				c.pendingCount.Add(1)
			} else if success {
				delete(c.data, key)
			} else {
				item.pending = false
				item.updatedWhilePersisting = false
				item.persistSnapshot = *new(T)
				item.persistSnapshotSet = false
				shouldTouch = true
			}
		}
	}
	c.mu.Unlock()

	if shouldTouch {
		if c.evictionCache != nil && !c.IsSaveDisabled() {
			var skip bool
			c.mu.RLock()
			if item, ok := c.data[key]; ok {
				skip = c.shouldSkipEviction(item.memoryItem)
			}
			c.mu.RUnlock()
			if skip {
				c.evictionCache.Delete(key)
			} else {
				c.evictionCache.Set(key, struct{}{})
			}
		}
	}
	c.pendingCount.Add(-1)
	c.persistWG.Done()
	if retry != nil {
		if c.enqueuePersist == nil {
			c.FinishPersist(retry.Key, retry.Generation, true)
		} else if !c.enqueuePersist(retry.Key, retry.Generation, retry.Reason) {
			c.RejectPersist(retry.Key, retry.Generation)
		}
	}
}

func (c *ResidencyCacheWithKey[K, T]) RejectPersist(key K, generation uint64) {
	if c == nil {
		return
	}

	c.mu.Lock()
	if item, ok := c.data[key]; ok {
		if item.pending && item.generation == generation {
			item.pending = false
			item.persisting = false
			item.updatedWhilePersisting = false
			item.persistSnapshot = *new(T)
			item.persistSnapshotSet = false
		}
	}
	c.mu.Unlock()
	c.pendingCount.Add(-1)
	c.persistWG.Done()
}

func (c *ResidencyCacheWithKey[K, T]) shouldSkipEviction(value T) bool {
	if c == nil || c.skipEviction == nil {
		return false
	}
	return c.skipEviction(value)
}

func (c *ResidencyCacheWithKey[K, T]) QueueAll(reason utils.EvictionReason) {
	if c == nil {
		return
	}
	tasks := c.MarkPending(nil, reason)
	for _, task := range tasks {
		if c.enqueuePersist == nil {
			c.FinishPersist(task.Key, task.Generation, true)
			continue
		}
		if !c.enqueuePersist(task.Key, task.Generation, task.Reason) {
			c.RejectPersist(task.Key, task.Generation)
		}
	}
}

func (c *ResidencyCacheWithKey[K, T]) QueueKeys(keys []K, reason utils.EvictionReason) {
	if c == nil || len(keys) == 0 {
		return
	}
	tasks := c.MarkPending(keys, reason)
	for _, task := range tasks {
		if c.enqueuePersist == nil {
			c.FinishPersist(task.Key, task.Generation, true)
			continue
		}
		if !c.enqueuePersist(task.Key, task.Generation, task.Reason) {
			c.RejectPersist(task.Key, task.Generation)
		}
	}
}

func (c *ResidencyCacheWithKey[K, T]) CoolDownKeys(keys []K, ttl time.Duration) {
	if c == nil || len(keys) == 0 || ttl <= 0 {
		return
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, key := range keys {
		item, ok := c.data[key]
		if !ok || item.pending || c.shouldSkipEviction(item.memoryItem) {
			continue
		}
		if c.evictionCache != nil {
			c.evictionCache.SetWithTTL(key, struct{}{}, ttl)
		}
	}
}

func (c *ResidencyCacheWithKey[K, T]) TrackKeys(keys []K) {
	if c == nil || len(keys) == 0 || c.evictionCache == nil || c.IsSaveDisabled() {
		return
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, key := range keys {
		item, ok := c.data[key]
		if !ok || item.pending || c.shouldSkipEviction(item.memoryItem) {
			continue
		}
		c.evictionCache.Set(key, struct{}{})
	}
}

func (c *ResidencyCacheWithKey[K, T]) MarkAllPending(reason utils.EvictionReason) []PersistRequest[K] {
	return c.MarkPending(nil, reason)
}

func (c *ResidencyCacheWithKey[K, T]) MarkPending(keys []K, reason utils.EvictionReason) []PersistRequest[K] {
	if c == nil {
		return nil
	}
	tasks := make([]PersistRequest[K], 0)
	c.mu.Lock()
	visit := func(key K) {
		item, ok := c.data[key]
		if !ok || item.pending {
			return
		}
		item.pending = true
		item.persisting = false
		item.updatedWhilePersisting = false
		item.persistSnapshot = *new(T)
		item.persistSnapshotSet = false
		item.generation++
		item.reason = reason
		tasks = append(tasks, PersistRequest[K]{Key: key, Generation: item.generation, Reason: reason})
		c.persistWG.Add(1)
		c.pendingCount.Add(1)
	}
	if len(keys) == 0 {
		for key := range c.data {
			visit(key)
		}
	} else {
		for _, key := range keys {
			visit(key)
		}
	}
	c.mu.Unlock()
	return tasks
}

func (c *ResidencyCacheWithKey[K, T]) Wait() {
	if c == nil {
		return
	}
	c.persistWG.Wait()
}

// WaitForKeysWithCancel waits until the given keys are no longer pending
// (persisted, rejected, or removed), without waiting for unrelated keys that
// were enqueued by later batches. This is the batch-scoped barrier used by
// mid-compile flushes: a per-unit drain can report a truthful resident_after
// and shrink the map as soon as its own batch settles, instead of waiting for
// the whole compile to finish.
func (c *ResidencyCacheWithKey[K, T]) WaitForKeysWithCancel(keys []K, cancel <-chan struct{}) bool {
	if c == nil || len(keys) == 0 {
		return true
	}
	if c.PendingCount() == 0 {
		return true
	}
	keySet := make(map[K]struct{}, len(keys))
	for _, key := range keys {
		keySet[key] = struct{}{}
	}
	for {
		c.mu.RLock()
		pending := false
		for key := range keySet {
			if item, ok := c.data[key]; ok && item.pending {
				pending = true
				break
			}
		}
		c.mu.RUnlock()
		if !pending {
			return true
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-cancel:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return false
		case <-timer.C:
		}
	}
}

// WaitWithCancel waits until all pending persistence requests settle. It is
// used by background drain callbacks so cache shutdown can cancel a wait when
// the saver has failed or the cache is intentionally being closed without a
// save. The ordinary Wait method remains the synchronous barrier for callers
// that require persistence completion.
func (c *ResidencyCacheWithKey[K, T]) WaitWithCancel(cancel <-chan struct{}) bool {
	if c == nil {
		return true
	}
	for c.PendingCount() > 0 {
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-cancel:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return false
		case <-timer.C:
		}
	}
	return true
}

func (c *ResidencyCacheWithKey[K, T]) GetAll() map[K]T {
	ret := make(map[K]T)
	if c == nil {
		return ret
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	for key, value := range c.data {
		ret[key] = value.memoryItem
	}
	return ret
}

func (c *ResidencyCacheWithKey[K, T]) Count() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}

func (c *ResidencyCacheWithKey[K, T]) PendingCount() int64 {
	if c == nil {
		return 0
	}
	return c.pendingCount.Load()
}

func (c *ResidencyCacheWithKey[K, T]) Keys() []K {
	if c == nil {
		return nil
	}
	keys := make([]K, 0)
	c.mu.RLock()
	for key := range c.data {
		keys = append(keys, key)
	}
	c.mu.RUnlock()
	return keys
}

// UpdateWhilePending replaces the memory value of a resident item
// WITHOUT clearing pending or incrementing generation. This allows
// content updates during in-flight save without breaking the save
// (SnapshotForPersist checks generation match). If the item does not
// exist or is not pending, this is a no-op.
func (c *ResidencyCacheWithKey[K, T]) UpdateWhilePending(key K, value T) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if item, ok := c.data[key]; ok && item.pending {
		item.memoryItem = value
		if item.persisting && item.persistSnapshotSet {
			item.updatedWhilePersisting = !reflect.DeepEqual(item.persistSnapshot, value)
		}
	}
	c.mu.Unlock()
}

func (c *ResidencyCacheWithKey[K, T]) Delete(key K) {
	c.DeleteWithoutSave(key)
}

func (c *ResidencyCacheWithKey[K, T]) DeleteWithoutSave(key K) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.data, key)
	c.mu.Unlock()
	if c.evictionCache != nil {
		c.evictionCache.Delete(key)
	}
}

func (c *ResidencyCacheWithKey[K, T]) ForEach(f func(K, T) bool) {
	if c == nil || f == nil {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	for key, value := range c.data {
		if !f(key, value.memoryItem) {
			return
		}
	}
}

func (c *ResidencyCacheWithKey[K, T]) Close() error {
	if c == nil {
		return nil
	}
	c.closed.Store(true)
	c.EnableSave()
	c.QueueAll(utils.EvictionReasonDeleted)
	c.Wait()
	remaining := c.Count()
	c.DisableSave()
	if c.evictionCache != nil {
		c.evictionCache.Close()
	}
	if remaining > 0 {
		return utils.Errorf("dbcache: %d resident items were not persisted on close", remaining)
	}
	return nil
}

func (c *ResidencyCacheWithKey[K, T]) CloseWithoutSave() {
	if c == nil {
		return
	}
	c.closed.Store(true)
	c.DisableSave()

	c.mu.Lock()
	c.data = make(map[K]*residentItem[K, T])
	c.mu.Unlock()

	if c.evictionCache != nil {
		c.evictionCache.Close()
	}
}

func (c *ResidencyCacheWithKey[K, T]) EnableSave() {
	if c == nil {
		return
	}
	if !c.saveDisabled.Swap(false) {
		return
	}
	c.restoreEvictionTracking()
}

func (c *ResidencyCacheWithKey[K, T]) DisableSave() {
	if c == nil {
		return
	}
	c.saveDisabled.Store(true)
}

func (c *ResidencyCacheWithKey[K, T]) IsSaveDisabled() bool {
	if c == nil {
		return false
	}
	return c.saveDisabled.Load()
}

func (c *ResidencyCacheWithKey[K, T]) IsClosed() bool {
	if c == nil {
		return false
	}
	return c.closed.Load()
}

func (c *ResidencyCacheWithKey[K, T]) MarkClosed() {
	if c == nil {
		return
	}
	c.closed.Store(true)
}

func (c *ResidencyCacheWithKey[K, T]) restoreEvictionTracking() {
	if c == nil || c.evictionCache == nil || c.IsSaveDisabled() {
		return
	}

	keys := make([]K, 0)
	c.mu.RLock()
	for key, item := range c.data {
		if item.pending || c.shouldSkipEviction(item.memoryItem) {
			continue
		}
		keys = append(keys, key)
	}
	c.mu.RUnlock()

	for _, key := range keys {
		c.evictionCache.Set(key, struct{}{})
	}
}

func (c *ResidencyCacheWithKey[K, T]) handleEviction(key K, reason utils.EvictionReason) {
	if c == nil {
		return
	}
	if c.evictionCache == nil {
		return
	}

	if c.IsSaveDisabled() {
		return
	}

	var generation uint64
	c.mu.Lock()
	item, ok := c.data[key]
	if !ok {
		c.mu.Unlock()
		return
	}
	if c.shouldSkipEviction(item.memoryItem) {
		c.mu.Unlock()
		if c.evictionCache != nil {
			c.evictionCache.Delete(key)
		}
		return
	}
	if item.pending {
		c.mu.Unlock()
		return
	}
	item.pending = true
	item.persisting = false
	item.updatedWhilePersisting = false
	item.persistSnapshot = *new(T)
	item.persistSnapshotSet = false
	item.generation++
	item.reason = reason
	generation = item.generation
	c.persistWG.Add(1)
	c.pendingCount.Add(1)
	c.mu.Unlock()

	if c.enqueuePersist == nil {
		c.FinishPersist(key, generation, true)
		return
	}
	if !c.enqueuePersist(key, generation, reason) {
		c.RejectPersist(key, generation)
	}
}

// ShrinkMap rebuilds the internal data map with a right-sized bucket
// array. Go's map delete() marks slots as empty but never shrinks the
// underlying bucket allocation, so after bulk eviction (e.g. FlushKeys
// removing 100K+ entries) the map retains its peak memory footprint.
// This method creates a new map sized to the current entry count and
// copies all remaining entries, allowing GC to reclaim the old buckets.
func (c *ResidencyCacheWithKey[K, T]) ShrinkMap() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.data) == 0 {
		c.data = make(map[K]*residentItem[K, T])
		return
	}
	newData := make(map[K]*residentItem[K, T], len(c.data))
	for k, v := range c.data {
		newData[k] = v
	}
	c.data = newData
}
