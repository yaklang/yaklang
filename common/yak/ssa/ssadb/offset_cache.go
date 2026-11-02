package ssadb

import (
	"strings"
	"sync"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils"
)

// offsetReadCacheCapacity bounds how many value-id snapshots stay cached. Each
// entry is an immutable copy of the rows for one (program, value) pair. Empty
// lookups are not stored, so a miss during compilation cannot hide a later
// insert.
const offsetReadCacheCapacity = 4096

// offsetCacheKey binds a cached snapshot to one program and to one generation
// of that program's offsets. Instruction ids are only unique inside a program,
// so a key without the program would let two concurrent compile projects answer
// each other's lookups. The generation bumps on every offset write batch, so a
// stale snapshot becomes unreachable instead of having to be hunted down.
type offsetCacheKey struct {
	program    string
	generation int64
	valueID    int64
}

var (
	initOffsetCacheOnce sync.Once
	offsetCache         *utils.CacheExWithKey[offsetCacheKey, []*IrOffset]

	offsetCacheGenerationMu sync.Mutex
	offsetCacheGenerations  = make(map[string]int64)
)

func offsetReadCache() *utils.CacheExWithKey[offsetCacheKey, []*IrOffset] {
	initOffsetCacheOnce.Do(func() {
		offsetCache = utils.NewCacheExWithKey[offsetCacheKey, []*IrOffset](
			utils.WithCacheCapacity(offsetReadCacheCapacity),
			utils.WithCacheTTL(irCacheTTL),
		)
	})
	return offsetCache
}

// offsetCacheGeneration returns the current offset generation of a program.
func offsetCacheGeneration(program string) int64 {
	offsetCacheGenerationMu.Lock()
	defer offsetCacheGenerationMu.Unlock()
	return offsetCacheGenerations[program]
}

// invalidateOffsetCache retires every snapshot of one program. Old keys stay
// unreadable and leave through the cache TTL or capacity, so no key has to be
// scanned. A program without a generation entry starts at zero.
func invalidateOffsetCache(programName string) {
	program := strings.TrimSpace(programName)
	if program == "" {
		return
	}
	offsetCacheGenerationMu.Lock()
	offsetCacheGenerations[program]++
	offsetCacheGenerationMu.Unlock()
}

// forgetOffsetCacheProgram drops the generation entry of a deleted program.
func forgetOffsetCacheProgram(programName string) {
	program := strings.TrimSpace(programName)
	if program == "" {
		return
	}
	invalidateOffsetCache(program)
	offsetCacheGenerationMu.Lock()
	delete(offsetCacheGenerations, program)
	offsetCacheGenerationMu.Unlock()
}

// PreloadOffsetsByValueIDs loads every current binding for the requested values
// in chunked native queries and retains non-empty snapshots for read-mode reuse.
// A value with no rows is left uncached. Legacy schemas return the native error
// so callers keep the per-variable GORM fallback.
func PreloadOffsetsByValueIDs(db *gorm.DB, programName string, valueIDs []int64) error {
	if db == nil {
		db = GetDB()
	}
	program := strings.TrimSpace(programName)
	rows, err := nativeGetOffsetsByValueIDs(db, program, valueIDs)
	if err != nil {
		return err
	}
	if program == "" {
		return nil
	}
	cache := offsetReadCache()
	generation := offsetCacheGeneration(program)
	grouped := make(map[int64][]*IrOffset)
	for _, row := range rows {
		if row == nil || row.ProgramName != program {
			continue
		}
		grouped[row.ValueID] = append(grouped[row.ValueID], cloneOffset(row))
	}
	for valueID, bindings := range grouped {
		if len(bindings) == 0 {
			continue
		}
		cache.Set(offsetCacheKey{program: program, generation: generation, valueID: valueID}, bindings)
	}
	return nil
}

func cachedOffsetsByVariable(programName, name string, valueID int64) ([]*IrOffset, bool) {
	program := strings.TrimSpace(programName)
	if program == "" || valueID <= 0 {
		return nil, false
	}
	key := offsetCacheKey{program: program, generation: offsetCacheGeneration(program), valueID: valueID}
	rows, ok := offsetReadCache().Get(key)
	if !ok {
		return nil, false
	}
	out := make([]*IrOffset, 0, len(rows))
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
