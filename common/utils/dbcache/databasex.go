package dbcache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pipeline"
)

type evictionRequest struct {
	key        int64
	generation uint64
	reason     utils.EvictionReason
}

type saveTask[D any] struct {
	request evictionRequest
	data    D
}

// PersistOutcome represents the terminal state of a persist request.
type PersistOutcome int

const (
	PersistSuccess       PersistOutcome = iota // save succeeded, item deleted from resident
	PersistFailed                              // save returned error
	PersistStale                               // SnapshotForPersist returned false (generation mismatch)
	PersistRejected                            // enqueuePersist rejected (queue full)
	PersistMarshalFailed                       // marshal callback returned error
)

// PersistSettlement is a single settlement event.
type PersistSettlement struct {
	Key        int64
	Generation uint64
	Outcome    PersistOutcome
}

// SettlementCallback is called when an item leaves the persist pipeline.
type SettlementCallback func(key int64, generation uint64, outcome PersistOutcome)

// SetSettlementCallback registers a callback called on every persist
// terminal event (success, failure, stale, rejected).
func (c *Cache[T, D]) SetSettlementCallback(cb SettlementCallback) {
	if c == nil {
		return
	}
	c.settlementMu.Lock()
	c.settlementCallback = cb
	c.settlementMu.Unlock()
}

// settle calls the settlement callback if registered.
func (c *Cache[T, D]) settle(key int64, generation uint64, outcome PersistOutcome) {
	if c == nil {
		return
	}
	c.settlementMu.RLock()
	cb := c.settlementCallback
	c.settlementMu.RUnlock()
	if cb != nil {
		cb(key, generation, outcome)
	}
}

// CacheStats is a compact debug snapshot of the residency cache and saver.
type CacheStats struct {
	ResidentCount int
	Saver         SaveStats
}

func (s CacheStats) String() string {
	return fmt.Sprintf("resident=%d %s", s.ResidentCount, s.Saver)
}

// Cache combines the residency cache with the marshal/save pipeline used by
// database-backed modes.
type Cache[T MemoryItem, D any] struct {
	resident     *ResidencyCacheWithKey[int64, T]
	marshalPipe  *pipeline.Pipe[evictionRequest, *saveTask[D]]
	saver        *Save[*saveTask[D]]
	marshal      MarshalFunc[T, D]
	save         SaveFunc[D]
	persistLimit int64

	closing atomic.Bool
	// finalDraining is set true by BeginFinalDrain to bypass persistLimit
	// backpressure during SaveToDatabase/final close. When true, enqueuePersist
	// accepts all items regardless of PendingCount, and MarkDirty's backpressure
	// loop is skipped. This ensures all remaining instructions are persisted
	// without PersistRejected during the final drain.
	finalDraining atomic.Bool
	// persistLimitBypass is set true by MarkDirtyAsync to bypass the
	// persistLimit PersistRejected check. Mid-compile flush must never
	// reject keys (rejected keys stay resident unsaved). The saver
	// goroutine drains in the background and evicts on success.
	persistLimitBypass atomic.Bool
	cancel             context.CancelFunc
	wg                 sync.WaitGroup

	// asyncDrainWG tracks the background drain/shrink callbacks started by
	// AsyncDrainAndShrink. A normal Barrier/Close waits for these callbacks so
	// they cannot observe a resident cache after it has been closed. The
	// cancellation channel is used by CloseWithoutSave and failed-save cleanup
	// to stop a drain that can no longer reach pendingCount == 0.
	asyncDrainMu         sync.Mutex
	asyncDrainWG         sync.WaitGroup
	asyncDrainClosed     bool
	asyncDrainCancel     chan struct{}
	asyncDrainCancelOnce sync.Once

	// settlementCallback is called whenever an item leaves the persist
	// pipeline (FinishPersist or RejectPersist).
	settlementMu       sync.RWMutex
	settlementCallback SettlementCallback
	closeOnce          sync.Once
	closeErr           error

	flushMu                   sync.Mutex
	flushRequestCount         int64
	flushDedupSkipped         int64
	flushEnqueueCount         int64
	flushSavedDelta           int64
	flushResidentBefore       int
	flushResidentAfter        int
	flushEnqueueDuration      time.Duration
	flushBackpressureDuration time.Duration
}

func NewCache[T MemoryItem, D any](
	ttl time.Duration,
	maxEntries int,
	marshal MarshalFunc[T, D],
	save SaveFunc[D],
	load LoadFunc[T],
	opt ...Option,
) *Cache[T, D] {
	cfg := NewConfig(opt...)
	ctx, cancel := context.WithCancel(cfg.ctx)
	skipEviction, _ := cfg.skipEviction.(func(T) bool)

	cache := &Cache[T, D]{
		cancel:           cancel,
		marshal:          marshal,
		save:             save,
		persistLimit:     int64(resolvePersistLimit(maxEntries, cfg.saveSize, cfg.persistLimit)),
		asyncDrainCancel: make(chan struct{}),
	}

	var resident *ResidencyCacheWithKey[int64, T]

	enqueuePersist := func(key int64, generation uint64, reason utils.EvictionReason) bool {
		if cache.marshalPipe == nil {
			resident.FinishPersist(key, generation, true)
			return true
		}
		if !cache.closing.Load() && !cache.finalDraining.Load() && !cache.persistLimitBypass.Load() && cache.persistLimit > 0 && resident.PendingCount() > cache.persistLimit {
			cache.settle(key, generation, PersistRejected)
			return false
		}
		cache.marshalPipe.Feed(evictionRequest{
			key:        key,
			generation: generation,
			reason:     reason,
		})
		return true
	}

	resident = NewResidencyCacheWithKey[int64, T](
		ttl,
		maxEntries,
		enqueuePersist,
		func(id int64) (T, error) {
			if load == nil {
				return *new(T), utils.Errorf("load function is not set")
			}
			return load(id)
		},
		skipEviction,
	)
	cache.resident = resident

	if marshal != nil || save != nil {
		cache.marshalPipe = pipeline.NewPipe(ctx, cfg.saveSize, func(request evictionRequest) (*saveTask[D], error) {
			value, ok := resident.SnapshotForPersist(request.key, request.generation)
			if !ok {
				resident.FinishPersist(request.key, request.generation, false)
				cache.settle(request.key, request.generation, PersistStale)
				return nil, nil
			}

			if marshal == nil {
				var zero D
				return &saveTask[D]{
					request: request,
					data:    zero,
				}, nil
			}

			data, err := marshal(value, request.reason)
			if err != nil {
				resident.FinishPersist(request.key, request.generation, false)
				cache.settle(request.key, request.generation, PersistMarshalFailed)
				return nil, err
			}

			return &saveTask[D]{
				request: request,
				data:    data,
			}, nil
		})

		cache.saver = NewSave(func(tasks []*saveTask[D]) error {
			return cache.handleSaveBatch(tasks, save)
		},
			WithContext(ctx),
			WithSaveSize(cfg.saveSize),
			WithSaveTimeout(cfg.saveTimeout),
			WithName(cfg.name),
		)

		cache.wg.Add(1)
		go func() {
			defer cache.wg.Done()
			for task := range cache.marshalPipe.Out() {
				if task == nil {
					continue
				}
				if cache.saver == nil || cache.saver.failed.Load() {
					resident.FinishPersist(task.request.key, task.request.generation, false)
					cache.settle(task.request.key, task.request.generation, PersistFailed)
					continue
				}
				cache.saver.Save(task)
			}
		}()
	}

	return cache
}

func (c *Cache[T, D]) Set(item T) {
	if c == nil || utils.IsNil(item) {
		return
	}
	if item.GetId() <= 0 {
		log.Errorf("dbcache got item without valid id")
		return
	}
	c.resident.Set(item.GetId(), item)
}

// UpdateWhilePending updates a resident item's value without
// clearing pending or incrementing generation. See ResidencyCacheWithKey.
func (c *Cache[T, D]) UpdateWhilePending(id int64, item T) {
	if c == nil || c.resident == nil {
		return
	}
	c.resident.UpdateWhilePending(id, item)
}

func (c *Cache[T, D]) Get(id int64) (T, bool) {
	if c == nil || c.resident == nil {
		return *new(T), false
	}
	return c.resident.Get(id)
}

// GetResident returns a value only if it is still in the resident cache; it
// never triggers a DB load. Save-path relation expansion uses this to avoid
// copying the entire resident map per batch.
func (c *Cache[T, D]) GetResident(id int64) (T, bool) {
	if c == nil || c.resident == nil {
		return *new(T), false
	}
	return c.resident.GetResident(id)
}

func (c *Cache[T, D]) Delete(id int64) {
	if c == nil || c.resident == nil {
		return
	}
	c.resident.DeleteWithoutSave(id)
}

func (c *Cache[T, D]) Count() int {
	if c == nil || c.resident == nil {
		return 0
	}
	return c.resident.Count()
}

func (c *Cache[T, D]) Stats() CacheStats {
	if c == nil {
		return CacheStats{}
	}
	stats := CacheStats{}
	if c.resident != nil {
		stats.ResidentCount = c.resident.Count()
	}
	if c.saver != nil {
		stats.Saver = c.saver.Stats()
	}
	return stats
}

func (c *Cache[T, D]) Evict(ids []int64, reason utils.EvictionReason) {
	if c == nil || c.resident == nil || len(ids) == 0 {
		return
	}
	c.resident.QueueKeys(ids, reason)
}

// Flush persists and evicts all currently resident items without closing the
// cache. It is intended for compile-unit boundaries where callers want DB
// backed data durable before moving to the next unit.
func (c *Cache[T, D]) Flush(reason utils.EvictionReason) {
	if c == nil || c.resident == nil {
		return
	}
	keys := c.resident.Keys()
	c.FlushKeys(keys, reason)
}

func (c *Cache[T, D]) FlushKeys(keys []int64, reason utils.EvictionReason) {
	if c == nil || c.resident == nil {
		return
	}
	start := time.Now()

	if len(keys) == 0 {
		if c.saver != nil {
			_ = c.saver.Flush()
		}
		// Track empty flush (drain request)
		c.flushMu.Lock()
		c.flushRequestCount++
		c.flushEnqueueDuration += time.Since(start)
		c.flushMu.Unlock()
		return
	}

	// Track resident before
	residentBefore := c.resident.Count()

	// Count how many keys are already pending (dedup)
	dedupSkipped := int64(0)
	c.resident.mu.RLock()
	for _, key := range keys {
		if item, ok := c.resident.data[key]; ok && item.pending {
			dedupSkipped++
		}
	}
	c.resident.mu.RUnlock()

	// Queue keys for persistence
	c.resident.QueueKeys(keys, reason)

	// Enqueue count = keys that were actually queued (not deduped)
	enqueueCount := int64(len(keys)) - dedupSkipped

	// Save stats before waiting
	savedBefore := int64(0)
	if c.saver != nil {
		savedBefore = int64(c.saver.Stats().BatchItemsTotal)
	}

	done := make(chan struct{})
	go func() {
		c.resident.Wait()
		close(done)
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if c.saver != nil {
			_ = c.saver.Flush()
		}
		select {
		case <-done:
			// Calculate saved delta
			savedAfter := int64(0)
			if c.saver != nil {
				savedAfter = int64(c.saver.Stats().BatchItemsTotal)
			}
			savedDelta := savedAfter - savedBefore

			// Reclaim map memory after bulk eviction.
			// Go's map delete() does not shrink the underlying bucket
			// array, so flushing 100K+ keys leaves a large empty map.
			// If more than half the keys were deleted, rebuild the map
			// with a right-sized bucket array to let GC reclaim memory.
			residentAfter := c.resident.Count()
			deleted := residentBefore - residentAfter
			if deleted > 0 && deleted > residentAfter {
				c.resident.ShrinkMap()
			}

			// Track metrics
			c.flushMu.Lock()
			c.flushRequestCount++
			c.flushDedupSkipped += dedupSkipped
			c.flushEnqueueCount += enqueueCount
			c.flushSavedDelta += savedDelta
			c.flushResidentBefore = residentBefore
			c.flushResidentAfter = residentAfter
			c.flushEnqueueDuration += time.Since(start)
			c.flushMu.Unlock()
			return
		case <-ticker.C:
		}
	}
}

func (c *Cache[T, D]) CoolDown(ids []int64, ttl time.Duration) {
	if c == nil || c.resident == nil || len(ids) == 0 || ttl <= 0 {
		return
	}
	c.resident.CoolDownKeys(ids, ttl)
}

func (c *Cache[T, D]) Track(ids []int64) {
	if c == nil || c.resident == nil || len(ids) == 0 {
		return
	}
	c.resident.TrackKeys(ids)
}

func (c *Cache[T, D]) GetAll() map[int64]T {
	if c == nil || c.resident == nil {
		return nil
	}
	return c.resident.GetAll()
}

func (c *Cache[T, D]) ForEach(f func(int64, T) bool) {
	if c == nil || c.resident == nil {
		return
	}
	c.resident.ForEach(f)
}

func (c *Cache[T, D]) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.closeErr = c.closeInternal()
	})
	return c.closeErr
}

func (c *Cache[T, D]) closeInternal() error {
	var closeErr error

	c.closing.Store(true)
	if c.saver != nil && c.saver.failed.Load() {
		// A failed saver may leave requests in the marshal pipe with their
		// resident wait-group entries still pending. Cancel background drain
		// callbacks before tearing down the pipeline; they must not wait forever
		// for work that can no longer be persisted.
		c.stopAsyncDrains(true, false)
		// Saver has failed — skip drain, just close.
		// Close marshalPipe first to stop the worker goroutine,
		// then wait for it to finish (wg.Wait), then close saver.
		// This prevents send-on-closed-channel race.
		if c.marshalPipe != nil {
			c.marshalPipe.Close()
		}
		c.wg.Wait() // wait for marshalPipe worker to stop
		if c.saver != nil {
			_ = c.saver.Close()
		}
		if c.resident != nil {
			c.resident.MarkClosed()
			c.resident.DisableSave()
			c.resident.CloseWithoutSave()
		}
		c.waitAsyncDrains()
		return c.saver.recordedErr()
	}
	// Normal close must let the completion callbacks observe the final
	// resident state before the cache is closed or cleared.
	c.stopAsyncDrains(false, true)
	if c.resident != nil {
		c.drainResidentForClose()
		c.resident.MarkClosed()
		c.resident.DisableSave()
	}

	if c.marshalPipe != nil {
		c.marshalPipe.Close()
		closeErr = utils.JoinErrors(closeErr, c.marshalPipe.Error())
	}
	c.wg.Wait()
	if c.saver != nil {
		closeErr = utils.JoinErrors(closeErr, c.saver.Close())
	}
	remaining := 0
	if c.resident != nil {
		c.resident.Wait()
		remaining = c.resident.Count()
		c.resident.CloseWithoutSave()
	}
	if c.cancel != nil {
		c.cancel()
	}
	if remaining > 0 {
		closeErr = utils.JoinErrors(closeErr, utils.Errorf("dbcache: %d resident items were not persisted on close", remaining))
	}
	return closeErr
}

func (c *Cache[T, D]) IsClosed() bool {
	if c == nil {
		return false
	}
	if c.closing.Load() {
		return true
	}
	return c.resident != nil && c.resident.IsClosed()
}

func (c *Cache[T, D]) drainResidentForClose() {
	if c == nil || c.resident == nil {
		return
	}
	const maxPasses = 8
	for pass := 0; pass < maxPasses; pass++ {
		remaining := c.resident.Count()
		if remaining == 0 {
			return
		}
		log.Infof("[dbcache-close] pass %d/%d: resident=%d items remaining", pass+1, maxPasses, remaining)
		c.Flush(utils.EvictionReasonDeleted)
		after := c.resident.Count()
		log.Infof("[dbcache-close] pass %d/%d: after flush resident=%d items remaining (evicted=%d)", pass+1, maxPasses, after, remaining-after)
		if c.marshalPipe != nil && c.marshalPipe.Error() != nil {
			return
		}
		if c.saver != nil && c.saver.failed.Load() {
			return
		}
		if after == 0 {
			return
		}
	}
	log.Warnf("[dbcache-close] maxPasses=%d exhausted, %d items still resident", maxPasses, c.resident.Count())
}

func (c *Cache[T, D]) enqueueCloseRequests() {
	if c == nil || c.resident == nil {
		return
	}
	keys := c.resident.Keys()
	if len(keys) == 0 {
		return
	}
	if c.marshalPipe == nil {
		c.resident.QueueKeys(keys, utils.EvictionReasonDeleted)
		return
	}

	limit := len(keys)
	if c.persistLimit > 0 && int(c.persistLimit) < limit {
		limit = int(c.persistLimit)
	}
	if limit <= 0 {
		limit = len(keys)
	}
	if c.persistLimit <= 0 || len(keys) <= limit {
		c.resident.QueueKeys(keys, utils.EvictionReasonDeleted)
		return
	}
	lowWatermark := limit / 2
	if lowWatermark <= 0 {
		lowWatermark = 1
	}

	for start := 0; start < len(keys); start += limit {
		end := start + limit
		if end > len(keys) {
			end = len(keys)
		}
		c.resident.QueueKeys(keys[start:end], utils.EvictionReasonDeleted)
		if end < len(keys) {
			c.waitPendingBelow(int64(lowWatermark))
		}
	}
}

func (c *Cache[T, D]) waitPendingBelow(limit int64) {
	if c == nil || c.resident == nil || limit <= 0 {
		return
	}
	for c.resident.PendingCount() > limit {
		time.Sleep(5 * time.Millisecond)
	}
}

func resolvePersistLimit(maxEntries, saveSize, override int) int {
	if override > 0 {
		return override
	}
	if maxEntries <= 0 && saveSize <= 0 {
		return 0
	}
	limit := maxEntries
	if limit <= 0 {
		limit = saveSize * 4
	}
	minLimit := saveSize * 4
	if minLimit > limit {
		limit = minLimit
	}
	if limit <= 0 {
		limit = saveSize
	}
	return max(limit, 512)
}

func (c *Cache[T, D]) CloseWithoutSave() {
	if c == nil {
		return
	}

	c.closing.Store(true)
	// This path is only safe after the caller has persisted what it needs, but
	// cancel any still-running observation so it cannot outlive the cache.
	c.stopAsyncDrains(true, true)
	if c.resident != nil {
		c.resident.CloseWithoutSave()
	}
	if c.marshalPipe != nil {
		c.marshalPipe.Close()
	}
	c.wg.Wait()
	if c.saver != nil {
		_ = c.saver.Close()
	}
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *Cache[T, D]) EnableSave() {
	if c == nil || c.resident == nil {
		return
	}
	c.resident.EnableSave()
}

func (c *Cache[T, D]) DisableSave() {
	if c == nil || c.resident == nil {
		return
	}
	c.resident.DisableSave()
}

func (c *Cache[T, D]) IsSaveDisabled() bool {
	if c == nil || c.resident == nil {
		return false
	}
	return c.resident.IsSaveDisabled()
}

func (c *Cache[T, D]) handleSaveBatch(tasks []*saveTask[D], save SaveFunc[D]) error {
	if c == nil || c.resident == nil {
		return nil
	}

	saveTasks := make([]*saveTask[D], 0, len(tasks))
	saveData := make([]D, 0, len(tasks))

	for _, task := range tasks {
		if task == nil {
			continue
		}
		if utils.IsNil(task.data) {
			c.resident.FinishPersist(task.request.key, task.request.generation, true)
			c.settle(task.request.key, task.request.generation, PersistSuccess)
			continue
		}
		saveTasks = append(saveTasks, task)
		saveData = append(saveData, task.data)
	}

	if len(saveTasks) == 0 {
		return nil
	}
	// Once the saver has failed, later batches must still be settled (the
	// resident persist WaitGroup was incremented when they were enqueued),
	// but they must not re-enter the real save callback. The marshal worker
	// also settles tasks after failure, so removing these entries here would
	// leave persistWG with unmatched Adds and Barrier/FlushKeys would hang.
	if c.saver != nil && c.saver.failed.Load() {
		for _, task := range saveTasks {
			c.resident.FinishPersist(task.request.key, task.request.generation, false)
			c.settle(task.request.key, task.request.generation, PersistFailed)
		}
		return nil
	}
	if save == nil {
		for _, task := range saveTasks {
			c.resident.FinishPersist(task.request.key, task.request.generation, false)
			c.settle(task.request.key, task.request.generation, PersistFailed)
		}
		return nil
	}

	if err := save(saveData); err != nil {
		log.Errorf("dbcache save failed: %v", err)
		for _, task := range saveTasks {
			c.resident.FinishPersist(task.request.key, task.request.generation, false)
			c.settle(task.request.key, task.request.generation, PersistFailed)
		}
		return err
	}
	for _, task := range saveTasks {
		c.resident.FinishPersist(task.request.key, task.request.generation, true)
		c.settle(task.request.key, task.request.generation, PersistSuccess)
	}
	return nil
}

// FlushStats is a per-flush observability snapshot. Currently a stub
// with zero values — will be populated by the async persist pipeline.
// This is a minimal test hook, not production observability.
type FlushStats struct {
	FlushRequestCount    int64
	DedupSkipped         int64
	EnqueueCount         int64
	SavedDelta           int64
	ResidentBefore       int
	ResidentAfter        int
	EnqueueDuration      time.Duration
	BackpressureDuration time.Duration
}

// FlushKeysStats returns flush observability metrics accumulated since
// cache creation. This is a minimal test hook; the async persist pipeline
// will replace this with per-flush structured stats.
func (c *Cache[T, D]) FlushKeysStats() FlushStats {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()
	return FlushStats{
		FlushRequestCount:    c.flushRequestCount,
		DedupSkipped:         c.flushDedupSkipped,
		EnqueueCount:         c.flushEnqueueCount,
		SavedDelta:           c.flushSavedDelta,
		ResidentBefore:       c.flushResidentBefore,
		ResidentAfter:        c.flushResidentAfter,
		EnqueueDuration:      c.flushEnqueueDuration,
		BackpressureDuration: c.flushBackpressureDuration,
	}
}

// MarkDirtyForTest marks keys as pending without waiting for the writer.
// This is a test-only method to verify dedup tracking. It does NOT block.
func (c *Cache[T, D]) MarkDirtyForTest(keys []int64, reason utils.EvictionReason) {
	if c == nil || c.resident == nil || len(keys) == 0 {
		return
	}

	// Count dedup: items already pending
	dedupSkipped := int64(0)
	c.resident.mu.RLock()
	for _, key := range keys {
		if item, ok := c.resident.data[key]; ok && item.pending {
			dedupSkipped++
		}
	}
	c.resident.mu.RUnlock()

	// Queue keys (non-blocking — MarkPending + enqueue)
	c.resident.QueueKeys(keys, reason)

	enqueueCount := int64(len(keys)) - dedupSkipped

	c.flushMu.Lock()
	c.flushRequestCount++
	c.flushDedupSkipped += dedupSkipped
	c.flushEnqueueCount += enqueueCount
	c.flushMu.Unlock()
}

// MarkDirty marks keys as dirty for asynchronous persistence without blocking.
// The compile thread calls this and immediately continues — the writer goroutine
// handles serialization + DB writes in the background. Same keys already pending
// are deduped (generation/dirty bit). When the queue depth exceeds the backpressure
// threshold, MarkDirty blocks until the writer catches up.
//
// This is the non-blocking replacement for FlushKeys in the compile hot path.
func (c *Cache[T, D]) MarkDirty(keys []int64, reason utils.EvictionReason) {
	if c == nil || c.resident == nil || len(keys) == 0 {
		return
	}
	start := time.Now()

	// Count dedup: items already pending
	dedupSkipped := int64(0)
	c.resident.mu.RLock()
	for _, key := range keys {
		if item, ok := c.resident.data[key]; ok && item.pending {
			dedupSkipped++
		}
	}
	c.resident.mu.RUnlock()

	// Backpressure: if too many items pending, wait for writer to catch up.
	// Do NOT call saver.Flush() here — it would block the compile thread.
	// Just poll until pending drops below the limit.
	backpressureStart := time.Now()
	if c.persistLimit > 0 && !c.finalDraining.Load() {
		for c.resident.PendingCount() > c.persistLimit && !c.closing.Load() {
			time.Sleep(time.Millisecond)
		}
	}
	backpressureDuration := time.Since(backpressureStart)

	// Queue keys for persistence (non-blocking — QueueKeys + enqueue)
	c.resident.QueueKeys(keys, reason)

	enqueueCount := int64(len(keys)) - dedupSkipped

	// Track metrics
	c.flushMu.Lock()
	c.flushRequestCount++
	c.flushDedupSkipped += dedupSkipped
	c.flushEnqueueCount += enqueueCount
	c.flushResidentBefore = c.resident.Count()
	c.flushEnqueueDuration += time.Since(start)
	c.flushBackpressureDuration += backpressureDuration
	c.flushMu.Unlock()
}

// MarkDirtyAsync marks keys as dirty for asynchronous persistence without
// blocking. Unlike MarkDirty it has NO persistLimit backpressure loop — the
// compile thread enqueues keys and returns immediately. The saver goroutine
// drains the pipeline in the background and evicts each saved entry via
// FinishPersist -> delete(c.data, key). Used by mid-compile flush so
// serialization + DB writes never block compilation.
func (c *Cache[T, D]) registerAsyncDrain() bool {
	if c == nil {
		return false
	}
	c.asyncDrainMu.Lock()
	defer c.asyncDrainMu.Unlock()
	if c.asyncDrainClosed || c.closing.Load() {
		return false
	}
	if c.asyncDrainCancel == nil {
		c.asyncDrainCancel = make(chan struct{})
	}
	c.asyncDrainWG.Add(1)
	return true
}

func (c *Cache[T, D]) stopAsyncDrains(cancel bool, wait bool) {
	if c == nil {
		return
	}
	c.asyncDrainMu.Lock()
	c.asyncDrainClosed = true
	if cancel && c.asyncDrainCancel != nil {
		c.asyncDrainCancelOnce.Do(func() {
			close(c.asyncDrainCancel)
		})
	}
	c.asyncDrainMu.Unlock()
	if wait {
		c.asyncDrainWG.Wait()
	}
}

func (c *Cache[T, D]) waitAsyncDrains() {
	if c == nil {
		return
	}
	c.asyncDrainWG.Wait()
}

// AsyncDrainAndShrink waits for all currently pending persistence to
// complete, then shrinks the resident map to reclaim memory. It runs in a
// background goroutine so compilation is never blocked. The optional callback
// is invoked only after persistence, eviction, and map shrinking complete;
// callers can use it to publish a truthful post-flush observation.
func (c *Cache[T, D]) AsyncDrainAndShrink(onComplete ...func()) {
	if c == nil || c.resident == nil || !c.registerAsyncDrain() {
		return
	}
	var callback func()
	if len(onComplete) > 0 {
		callback = onComplete[0]
	}
	c.asyncDrainMu.Lock()
	cancel := c.asyncDrainCancel
	c.asyncDrainMu.Unlock()
	go func() {
		defer c.asyncDrainWG.Done()
		// Wait for all pending saves to complete (evict via FinishPersist).
		if !c.resident.WaitWithCancel(cancel) {
			return
		}
		// Reclaim map memory after bulk eviction.
		c.resident.ShrinkMap()
		if callback != nil {
			callback()
		}
	}()
}

// AsyncDrainKeysAndShrink is the batch-scoped variant of AsyncDrainAndShrink:
// it waits only for the supplied keys to settle (persist/reject/remove), then
// shrinks the resident map and runs the callback. Mid-compile flushes use this
// so a later unit's flush cannot delay the previous unit's memory reclamation
// or its post-flush observability callback.
func (c *Cache[T, D]) AsyncDrainKeysAndShrink(keys []int64, onComplete ...func()) {
	if c == nil || c.resident == nil || len(keys) == 0 || !c.registerAsyncDrain() {
		return
	}
	var callback func()
	if len(onComplete) > 0 {
		callback = onComplete[0]
	}
	c.asyncDrainMu.Lock()
	cancel := c.asyncDrainCancel
	c.asyncDrainMu.Unlock()
	go func() {
		defer c.asyncDrainWG.Done()
		if !c.resident.WaitForKeysWithCancel(keys, cancel) {
			return
		}
		c.resident.ShrinkMap()
		if callback != nil {
			callback()
		}
	}()
}

func (c *Cache[T, D]) MarkDirtyAsync(keys []int64, reason utils.EvictionReason) {
	if c == nil || c.resident == nil || len(keys) == 0 {
		return
	}
	start := time.Now()
	c.persistLimitBypass.Store(true)

	// Count dedup: items already pending
	dedupSkipped := int64(0)
	c.resident.mu.RLock()
	for _, key := range keys {
		if item, ok := c.resident.data[key]; ok && item.pending {
			dedupSkipped++
		}
	}
	c.resident.mu.RUnlock()

	// Queue keys (non-blocking — MarkPending + enqueue)
	c.resident.QueueKeys(keys, reason)

	enqueueCount := int64(len(keys)) - dedupSkipped

	c.flushMu.Lock()
	c.flushRequestCount++
	c.flushDedupSkipped += dedupSkipped
	c.flushEnqueueCount += enqueueCount
	c.flushResidentBefore = c.resident.Count()
	c.flushEnqueueDuration += time.Since(start)
	c.flushMu.Unlock()
}

// Barrier waits for all pending persistence operations to complete.
// This is the synchronous wait point used by SaveToDatabase (final barrier)
// and other close/checkpoint operations. Returns the first write error if any.
//
// Barrier does NOT flush resident items — it only waits for items already
// enqueued via MarkDirty to be persisted. Use Flush for full flush.
func (c *Cache[T, D]) Barrier() error {
	if c == nil || c.resident == nil {
		return nil
	}

	// Check if saver has already failed — return error immediately.
	// Do NOT call resident.Wait() — items pending in the marshal pipe
	// will never be FinishPersist'd (saver rejected them), so Wait()
	// would block forever. The compile loop should stop on this error.
	if c.saver != nil && c.saver.failed.Load() {
		return c.saver.recordedErr()
	}

	// A marshal request becomes a persistWG entry before it reaches the saver.
	// Draining the saver only after resident.Wait creates a cycle for a partial
	// batch: the wait needs FinishPersist, while FinishPersist needs a saver
	// flush. Keep flushing until all currently pending requests settle; this
	// also covers requests that cross marshalPipe just after an earlier flush.
	if c.saver != nil {
		for c.resident.PendingCount() > 0 {
			if err := c.saver.Flush(); err != nil {
				c.waitAsyncDrains()
				return err
			}
			if c.resident.PendingCount() > 0 {
				time.Sleep(time.Millisecond)
			}
		}
	}

	// Preserve the WaitGroup boundary after observing pendingCount == 0. As
	// with the old Barrier contract, callers must not enqueue new work
	// concurrently with this final synchronization point.
	c.resident.Wait()
	// The persistence barrier also establishes the lifecycle boundary for
	// post-flush callbacks. SaveToDatabase may close the writer immediately
	// after Barrier returns, so do not let a callback race that close.
	c.waitAsyncDrains()

	// Return any recorded error from the saver
	if c.saver != nil {
		return c.saver.recordedErr()
	}
	return nil
}

// BeginFinalDrain transitions the cache into final-draining mode.
// After this call, enqueuePersist bypasses the persistLimit backpressure
// check, and MarkDirty's backpressure loop is skipped. This ensures that
// all remaining resident items can be enqueued for persistence without
// PersistRejected during SaveToDatabase/final close.
//
// This does NOT disable persistLimit globally — compile-time MarkDirty
// calls before BeginFinalDrain still respect the limit. Only the final
// drain phase is exempt.
//
// Caller must call this before the final flush/Barrier sequence in the
// close path. It is idempotent.
func (c *Cache[T, D]) BeginFinalDrain() {
	if c == nil {
		return
	}
	c.finalDraining.Store(true)
}

// IsFinalDraining returns true if BeginFinalDrain has been called.
func (c *Cache[T, D]) IsFinalDraining() bool {
	if c == nil {
		return false
	}
	return c.finalDraining.Load()
}
