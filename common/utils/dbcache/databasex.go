package dbcache

import (
	"context"
	"fmt"
	"sync"
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

type CacheStats struct {
	ResidentCount int
	Saver         SaveStats
}

func (s CacheStats) Show() string {
	return fmt.Sprintf("resident=%d %s", s.ResidentCount, s.Saver.Show())
}

type Cache[T MemoryItem, D any] struct {
	resident     *ResidencyCacheWithKey[int64, T]
	marshalPipe  *pipeline.Pipe[evictionRequest, *saveTask[D]]
	saver        *Save[*saveTask[D]]
	marshal      MarshalFunc[T, D]
	save         SaveFunc[D]
	persistLimit int64

	cancel context.CancelFunc
	wg     sync.WaitGroup
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
		cancel:       cancel,
		marshal:      marshal,
		save:         save,
		persistLimit: int64(resolvePersistLimit(maxEntries, cfg.saveSize, cfg.persistLimit)),
	}

	var resident *ResidencyCacheWithKey[int64, T]

	enqueuePersist := func(key int64, generation uint64, reason utils.EvictionReason) bool {
		if cache.marshalPipe == nil {
			resident.FinishPersist(key, generation, true)
			return true
		}
		if cache.persistLimit > 0 && resident.PendingCount() > cache.persistLimit {
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
				return nil, err
			}

			return &saveTask[D]{
				request: request,
				data:    data,
			}, nil
		})

		cache.saver = NewSave(func(tasks []*saveTask[D]) {
			cache.handleSaveBatch(tasks, save)
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

func (c *Cache[T, D]) Get(id int64) (T, bool) {
	if c == nil || c.resident == nil {
		return *new(T), false
	}
	return c.resident.Get(id)
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

func (c *Cache[T, D]) Close() {
	if c == nil {
		return
	}

	if c.resident != nil {
		c.resident.MarkClosed()
		c.resident.DisableSave()
	}

	if c.marshalPipe != nil {
		c.enqueueCloseRequests()
		c.marshalPipe.Close()
	}
	c.wg.Wait()
	if c.saver != nil {
		c.saver.Close()
	}
	if c.resident != nil {
		c.resident.Wait()
		c.resident.CloseWithoutSave()
	}
	if c.cancel != nil {
		c.cancel()
	}
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

	if c.resident != nil {
		c.resident.CloseWithoutSave()
	}
	if c.marshalPipe != nil {
		c.marshalPipe.Close()
	}
	c.wg.Wait()
	if c.saver != nil {
		c.saver.Close()
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

func (c *Cache[T, D]) handleSaveBatch(tasks []*saveTask[D], save SaveFunc[D]) {
	if c == nil || c.resident == nil {
		return
	}

	saveTasks := make([]*saveTask[D], 0, len(tasks))
	saveData := make([]D, 0, len(tasks))

	for _, task := range tasks {
		if task == nil {
			continue
		}
		if utils.IsNil(task.data) {
			c.resident.FinishPersist(task.request.key, task.request.generation, true)
			continue
		}
		saveTasks = append(saveTasks, task)
		saveData = append(saveData, task.data)
	}

	if len(saveTasks) == 0 {
		return
	}
	if save == nil {
		for _, task := range saveTasks {
			c.resident.FinishPersist(task.request.key, task.request.generation, false)
		}
		return
	}

	if err := save(saveData); err != nil {
		log.Errorf("dbcache save failed: %v", err)
		for _, task := range saveTasks {
			c.resident.FinishPersist(task.request.key, task.request.generation, false)
		}
		return
	}
	for _, task := range saveTasks {
		c.resident.FinishPersist(task.request.key, task.request.generation, true)
	}
}
