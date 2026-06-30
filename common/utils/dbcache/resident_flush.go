package dbcache

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// ResidentFlushCache is a resident (non-evicting) cache with a synchronous,
// incremental, batched flush. It is used by callers that keep every item
// resident until Close and need to hand them to a save function in batches.
//
// Flush(false) (an incremental flush) persists items that have not been
// persisted yet via the insert path. Flush(true) (the close flush) persists
// everything: upsert if any incremental flush has already run, otherwise
// insert. The marshal callback receives the resulting updateExisting flag so
// it can mark each record accordingly.
//
// Flush is exclusive: callers must not mutate the cache concurrently with a
// Flush/Close call (same contract as Save.Flush).
type ResidentFlushCache[K comparable, T any, D any] struct {
	data      *utils.SafeMapWithKey[K, T]
	persisted map[K]struct{}
	mu        sync.Mutex // guards persisted; Flush is exclusive

	marshal   func(T, bool) (D, bool, error) // (item, updateExisting) -> (record, ok, err)
	saveBatch func([]D) error
	saveSize  int
}

// NewResidentFlushCache creates a ResidentFlushCache.
//
//   - marshal turns an item into its persist record. ok=false means skip the
//     item silently (e.g. no DB row to write); err means a failure to
//     accumulate and return. The bool argument is the updateExisting flag the
//     cache computed for this Flush.
//   - saveBatch persists a slice of records; it must branch per-record on the
//     UpdateExisting flag carried in D, since a single Flush produces a
//     uniformly insert or uniformly upsert batch.
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
		marshal:   marshal,
		saveBatch: saveBatch,
		saveSize:  saveSize,
	}
}

// Map returns the underlying resident map so callers can reuse existing
// Set/Get/Delete/Count/GetAll/ForEach code paths against the same storage.
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
// On an incremental flush (final=false), items already persisted are skipped;
// new items are handed to saveBatch and, on success, recorded as persisted.
//
// On the close flush (final=true), nothing is skipped: if any incremental
// flush has run (persisted non-empty) every record is marshaled with
// updateExisting=true (upsert); otherwise updateExisting=false (insert). This
// preserves the insert-vs-upsert distinction without a separate close path.
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
	upsert := final && len(c.persisted) > 0
	c.mu.Unlock()

	batch := make([]D, 0, c.saveSize)
	keys := make([]K, 0, c.saveSize)
	var flushErr error
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := c.saveBatch(batch); err != nil {
			flushErr = utils.JoinErrors(flushErr, err)
		} else if !final {
			c.mu.Lock()
			for _, k := range keys {
				c.persisted[k] = struct{}{}
			}
			c.mu.Unlock()
		}
		batch = batch[:0]
		keys = keys[:0]
	}

	c.data.ForEach(func(key K, value T) bool {
		c.mu.Lock()
		_, already := c.persisted[key]
		c.mu.Unlock()
		if !final && already {
			return true
		}
		rec, ok, err := c.marshal(value, upsert)
		if err != nil {
			flushErr = utils.JoinErrors(flushErr, err)
			return true
		}
		if !ok {
			return true
		}
		batch = append(batch, rec)
		keys = append(keys, key)
		if len(batch) >= c.saveSize {
			flush()
		}
		return true
	})
	flush()
	return flushErr
}

// Close is equivalent to Flush(true).
func (c *ResidentFlushCache[K, T, D]) Close() error {
	return c.Flush(true)
}
