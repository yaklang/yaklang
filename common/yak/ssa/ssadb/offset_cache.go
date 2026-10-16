package ssadb

import (
	"strings"
	"sync"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

// offsetReadCacheCapacity bounds how many value-id snapshots a read-only
// program keeps. Entries are immutable copies. Empty lookups are not stored,
// so a miss during compilation cannot hide a later insert.
const offsetReadCacheCapacity = 4096

type offsetProgramCache struct {
	rows *utils.CacheExWithKey[int64, []*IrOffset]
}

var (
	initOffsetCacheOnce sync.Once
	offsetCaches        *utils.CacheEx[*offsetProgramCache]
)

func newOffsetCaches() *utils.CacheEx[*offsetProgramCache] {
	parent := utils.NewCacheEx[*offsetProgramCache](utils.WithCacheTTL(ProgramCacheTTL))
	parent.SetExpirationCallback(func(_ string, child *offsetProgramCache, _ utils.EvictionReason) {
		if child != nil && child.rows != nil {
			child.rows.Close()
		}
	})
	return parent
}

func offsetCacheFor(programName string) *offsetProgramCache {
	programName = strings.TrimSpace(programName)
	if programName == "" {
		return nil
	}
	initOffsetCacheOnce.Do(func() {
		offsetCaches = newOffsetCaches()
	})
	if child, ok := offsetCaches.Get(programName); ok {
		return child
	}
	programRowCacheMu.Lock()
	defer programRowCacheMu.Unlock()
	if child, ok := offsetCaches.Get(programName); ok {
		return child
	}
	child := &offsetProgramCache{
		rows: utils.NewCacheExWithKey[int64, []*IrOffset](
			utils.WithCacheCapacity(offsetReadCacheCapacity),
			utils.WithCacheTTL(irCacheTTL),
		),
	}
	offsetCaches.Set(programName, child)
	return child
}

func invalidateOffsetCache(programName string) {
	programName = strings.TrimSpace(programName)
	if programName == "" || offsetCaches == nil {
		return
	}
	offsetCaches.Delete(programName)
}

// PreloadOffsetsByValueIDs loads every current binding for the requested values
// in chunked native queries and retains non-empty snapshots for read-mode reuse.
// A value with no rows is left uncached. Legacy schemas return the native error
// so callers keep the per-variable GORM fallback.
func PreloadOffsetsByValueIDs(db *gorm.DB, programName string, valueIDs []int64) error {
	if db == nil {
		db = GetDB()
	}
	rows, err := nativeGetOffsetsByValueIDs(db, programName, valueIDs)
	if err != nil {
		return err
	}
	cache := offsetCacheFor(programName)
	if cache == nil {
		return nil
	}
	grouped := make(map[int64][]*IrOffset)
	for _, row := range rows {
		if row == nil {
			continue
		}
		grouped[row.ValueID] = append(grouped[row.ValueID], cloneOffset(row))
	}
	for valueID, bindings := range grouped {
		if len(bindings) == 0 {
			continue
		}
		cache.rows.Set(valueID, bindings)
	}
	return nil
}

func cachedOffsetsByVariable(programName, name string, valueID int64) ([]*IrOffset, bool) {
	programName = strings.TrimSpace(programName)
	if programName == "" || valueID <= 0 || offsetCaches == nil {
		return nil, false
	}
	cache, ok := offsetCaches.Get(programName)
	if !ok || cache == nil {
		return nil, false
	}
	rows, ok := cache.rows.Get(valueID)
	if !ok {
		return nil, false
	}
	out := make([]*IrOffset, 0)
	for _, row := range rows {
		if row != nil && row.VariableName == name {
			out = append(out, cloneOffset(row))
		}
	}
	return out, true
}

func cloneOffset(row *IrOffset) *IrOffset {
	if row == nil {
		return nil
	}
	copied := *row
	return &copied
}
