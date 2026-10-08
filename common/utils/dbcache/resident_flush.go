package dbcache

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// ResidentFlushCache has a synchronous, incremental, batched dirty-key flush.
// Set/MarkDirty publish mutations; clean reads do not enqueue writes. Both
// incremental and final flushes visit only dirty keys, using each key's saved
// state to choose INSERT or UPDATE. A clean flush returns without walking data.
//
// Flush is exclusive: callers must not mutate the cache concurrently with a
// Flush/Close call (same contract as Save.Flush).
type ResidentFlushCache[K comparable, T any, D any] struct {
	data      *utils.SafeMapWithKey[K, T]
	persisted map[K]struct{}
	dirty     map[K]uint64
	version   uint64
	mu        sync.Mutex // guards persisted/dirty; Flush is exclusive

	marshal   func(T, bool) (D, bool, error) // (item, updateExisting) -> (record, ok, err)
	saveBatch func([]D) error
	saveSize  int
	onSaved   func(K, T)
}

// NewResidentFlushCache creates a ResidentFlushCache.
//
//   - marshal turns an item into its persist record. ok=false means skip the
//     item silently (e.g. no DB row to write); err means a failure to
//     accumulate and return. The bool argument is the updateExisting flag the
//     cache computed for this Flush.
//   - saveBatch persists a slice of records; it must branch per-record on the
//     UpdateExisting flag carried in D, since a single Flush produces a
//     mixed insert/update batch.
func NewResidentFlushCache[K comparable, T any, D any](
	saveSize int,
	marshal func(T, bool) (D, bool, error),
	saveBatch func([]D) error,
) *ResidentFlushCache[K, T, D] {
	if saveSize <= 0 {
		saveSize = 1
	}
	return &ResidentFlushCache[K, T, D]{
		data:      utils.NewSafeMapWithKey[K, T](),
		persisted: make(map[K]struct{}),
		dirty:     make(map[K]uint64),
		marshal:   marshal,
		saveBatch: saveBatch,
		saveSize:  saveSize,
	}
}

// Map exposes storage for clean reads/reloads. Mutations must use Set/MarkDirty.
func (c *ResidentFlushCache[K, T, D]) Map() *utils.SafeMapWithKey[K, T] {
	if c == nil {
		return nil
	}
	return c.data
}

func (c *ResidentFlushCache[K, T, D]) Set(key K, value T) {
	if c == nil || c.data == nil {
		return
	}
	c.data.Set(key, value)
	c.MarkDirty(key)
}

// MarkDirty queues a key in constant time. Generations prevent a successful
// save from clearing a mutation published while that save was in progress.
func (c *ResidentFlushCache[K, T, D]) MarkDirty(key K) {
	c.mu.Lock()
	c.version++
	c.dirty[key] = c.version
	c.mu.Unlock()
}

func (c *ResidentFlushCache[K, T, D]) DirtyCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.dirty)
}

// SetOnSaved installs a callback for successfully settled entries. It may
// release clean residency, but must not discard entries that became dirty.
func (c *ResidentFlushCache[K, T, D]) SetOnSaved(callback func(K, T)) {
	c.onSaved = callback
}

func (c *ResidentFlushCache[K, T, D]) DeleteClean(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, dirty := c.dirty[key]; dirty {
		return false
	}
	c.data.Delete(key)
	return true
}

func (c *ResidentFlushCache[K, T, D]) Get(key K) (T, bool) {
	if c == nil || c.data == nil {
		var zero T
		return zero, false
	}
	return c.data.Get(key)
}

func (c *ResidentFlushCache[K, T, D]) Delete(key K) {
	if c == nil {
		return
	}
	c.data.Delete(key)
	c.mu.Lock()
	delete(c.dirty, key)
	c.mu.Unlock()
}

func (c *ResidentFlushCache[K, T, D]) Count() int {
	if c == nil || c.data == nil {
		return 0
	}
	return c.data.Count()
}

func (c *ResidentFlushCache[K, T, D]) GetAll() map[K]T {
	if c == nil || c.data == nil {
		return nil
	}
	return c.data.GetAll()
}

func (c *ResidentFlushCache[K, T, D]) ForEach(f func(K, T) bool) {
	if c == nil || c.data == nil || f == nil {
		return
	}
	c.data.ForEach(f)
}

// PersistedCount returns how many resident items have been recorded as
// persisted by a successful incremental Flush(false). Useful for telemetry.
func (c *ResidentFlushCache[K, T, D]) PersistedCount() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.persisted)
}

// Flush synchronously persists resident items in batches.
//
// Both incremental and final flushes save only dirty keys. A successful save
// clears only the observed generation; failures and newer mutations remain
// dirty. The final argument is retained for compatibility with existing callers.
//
// marshal/saveBatch errors are joined and returned; a record with ok=false is
// silently skipped.
func (c *ResidentFlushCache[K, T, D]) Flush(final bool) error {
	if c == nil || c.data == nil || c.saveBatch == nil {
		return nil
	}
	c.mu.Lock()
	if c.persisted == nil {
		c.persisted = make(map[K]struct{})
	}
	if len(c.dirty) == 0 {
		c.mu.Unlock()
		return nil
	}
	selected := make(map[K]uint64, len(c.dirty))
	for key, version := range c.dirty {
		selected[key] = version
	}
	c.mu.Unlock()

	batch := make([]D, 0, c.saveSize)
	keys := make([]K, 0, c.saveSize)
	versions := make([]uint64, 0, c.saveSize)
	var flushErr error
	settle := func(key K, version uint64) {
		c.mu.Lock()
		c.persisted[key] = struct{}{}
		if c.dirty[key] == version {
			delete(c.dirty, key)
		}
		c.mu.Unlock()
		if c.onSaved != nil {
			if value, ok := c.data.Get(key); ok {
				c.onSaved(key, value)
			}
		}
	}
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := c.saveBatch(batch); err != nil {
			flushErr = utils.JoinErrors(flushErr, err)
		} else {
			for i, key := range keys {
				settle(key, versions[i])
			}
		}
		clear(batch)
		batch = batch[:0]
		keys = keys[:0]
		versions = versions[:0]
	}

	for key, version := range selected {
		value, exists := c.data.Get(key)
		if !exists {
			c.mu.Lock()
			if c.dirty[key] == version {
				delete(c.dirty, key)
			}
			c.mu.Unlock()
			continue
		}
		c.mu.Lock()
		_, already := c.persisted[key]
		c.mu.Unlock()
		rec, ok, err := c.marshal(value, already)
		if err != nil {
			flushErr = utils.JoinErrors(flushErr, err)
			continue
		}
		if !ok {
			settle(key, version)
			continue
		}
		batch = append(batch, rec)
		keys = append(keys, key)
		versions = append(versions, version)
		if len(batch) >= c.saveSize {
			flush()
		}
	}
	flush()
	c.mu.Lock()
	if len(c.dirty) == 0 {
		// A large unit must not retain its empty dirty-map backing storage.
		c.dirty = make(map[K]uint64)
	}
	c.mu.Unlock()
	return flushErr
}

// Close is equivalent to Flush(true).
func (c *ResidentFlushCache[K, T, D]) Close() error {
	return c.Flush(true)
}
