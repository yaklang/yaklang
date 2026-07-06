package ssa

import (
	"runtime"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"go.uber.org/atomic"
)

type ProgramCacheKind int

const (
	_ ProgramCacheKind = iota
	ProgramCacheMemory
	ProgramCacheDBRead
	ProgramCacheDBWrite
)

type ProgramCache struct {
	program *Program
	db      *gorm.DB

	instructions *instructionStore
	types        *typeStore
	sources      *sourceStore
	indexes      *indexStore

	// Track last flush statistics for telemetry
	lastReleasedEditors int
}

func NewDBCache(cfg *ssaconfig.Config, prog *Program, databaseKind ProgramCacheKind, fileSize int) *ProgramCache {
	cfg = ensureProgramConfig(cfg)
	cache := &ProgramCache{
		program: prog,
	}

	var programName string
	if databaseKind != ProgramCacheMemory {
		programName = prog.GetApplication().GetProgramName()
		cache.db = ssadb.GetDB().Where("program_name = ?", programName)
	}
	if databaseKind != ProgramCacheMemory && instructionCacheDebugEnabled() {
		cacheTTL, cacheMax := resolveInstructionCacheSettings(cfg)
		log.Debugf("[ssa-ir-cache] init: program=%s ttl=%s max=%d kind=%d",
			programName, cacheTTL, cacheMax, databaseKind,
		)
	}

	saveSize := min(max(fileSize*5, defaultSaveSize), maxSaveSize)
	log.Debugf("asyncdb Channel: ReSetSize: fileSize(%d) saveSize(%d)", fileSize, saveSize)

	cache.sources = newSourceStore(prog, databaseKind, cache.db)
	cache.indexes = newIndexStore(cfg, prog, databaseKind, cache.db, saveSize/2)
	cache.types = newTypeStore(cfg, prog, databaseKind, cache.db, programName, saveSize)
	cache.instructions = newInstructionStore(cfg, prog, databaseKind, cache.db, saveSize)
	return cache
}

func (c *ProgramCache) HaveDatabaseBackend() bool {
	return c != nil && c.db != nil
}

func (c *ProgramCache) DebugDB() {
	if c == nil || c.db == nil {
		return
	}
	c.db = c.db.Debug()
}

func (c *ProgramCache) DisableInstructionSpill() {
	if c == nil || !c.HaveDatabaseBackend() || c.instructions == nil {
		return
	}
	c.instructions.DisableSpill()
}

func (c *ProgramCache) EnableInstructionSpill() {
	if c == nil || !c.HaveDatabaseBackend() || c.instructions == nil {
		return
	}
	c.instructions.EnableSpill()
}

func (c *ProgramCache) IsInstructionSpillDisabled() bool {
	if c == nil || !c.HaveDatabaseBackend() || c.instructions == nil {
		return false
	}
	return c.instructions.IsSpillDisabled()
}

func (c *ProgramCache) IsClosed() bool {
	if c == nil || c.instructions == nil {
		return false
	}
	return c.instructions.IsClosed()
}

func (c *ProgramCache) SetInstruction(inst Instruction) {
	if utils.IsNil(inst) {
		log.Errorf("BUG: SetInstruction called with nil instruction")
		return
	}
	if c != nil && c.indexes != nil {
		c.indexes.AddInstructionOffsets(inst)
	}
	if c != nil && c.instructions != nil {
		c.instructions.Set(inst)
	}
}

func (c *ProgramCache) DeleteInstruction(inst Instruction) {
	if c == nil || c.instructions == nil || utils.IsNil(inst) {
		return
	}
	c.instructions.Delete(inst.GetId())
}

func (c *ProgramCache) GetInstruction(id int64) Instruction {
	if c == nil || c.instructions == nil || id == 0 {
		return nil
	}
	return c.instructions.Get(id)
}

func (c *ProgramCache) PreloadInstructionsByIDsFast(ids []int64) {
	if c == nil || c.instructions == nil {
		return
	}
	c.instructions.PreloadByIDsFast(ids)
}

func (c *ProgramCache) AddConst(inst Instruction) {
	if c == nil || c.indexes == nil {
		return
	}
	c.indexes.AddConst(inst)
}

func (c *ProgramCache) AddVariable(name string, inst Instruction) {
	if c == nil || c.indexes == nil {
		return
	}
	c.indexes.AddVariable(name, inst)
}

func (c *ProgramCache) RemoveVariable(name string, inst Instruction) {
	if c == nil || c.indexes == nil {
		return
	}
	c.indexes.RemoveVariable(name, inst)
}

func (c *ProgramCache) AddClassInstance(name string, inst Instruction) {
	if c == nil || c.indexes == nil {
		return
	}
	c.indexes.AddClassInstance(name, inst)
}

func (c *ProgramCache) SaveToDatabase(cb ...func(int)) error {
	if !c.HaveDatabaseBackend() {
		return nil
	}
	progress := func(int) {}
	if len(cb) > 0 && cb[0] != nil {
		progress = cb[0]
	}

	steps := []func() error{
		func() error {
			if c.types != nil {
				c.types.close()
				log.Infof("Type Cache closed")
			}
			return nil
		},
		func() error {
			if c.indexes != nil {
				c.indexes.Close()
			}
			return nil
		},
		func() error {
			if c.instructions != nil {
				if err := c.instructions.Close(progress); err != nil {
					return err
				}
				log.Infof("Instruction cache closed")
			}
			return nil
		},
		func() error {
			if c.sources != nil {
				c.sources.Close()
			}
			return nil
		},
		func() error {
			if c.program != nil && c.instructions != nil {
				stats := c.instructions.Stats()
				log.Debugf("[ssa-ir-cache-saver] program=%s %s", c.program.GetProgramName(), stats)
			}
			return nil
		},
	}
	return c.diagnosticsTrackErr("ssa.ProgramCache.SaveToDatabase", steps...)
}

func (c *ProgramCache) FlushCompileUnit(unitKey string) {
	if c == nil || !c.HaveDatabaseBackend() {
		return
	}
	// Per-batch flush bounds memory by spilling ORDINARY instructions to DB
	// (flushCompileUnitWriter keeps BasicBlocks + Function/Parameter/FreeValue
	// boundary instructions resident, so block ScopeTable survives for lazy
	// builds). Stores (indexes/types/sources) are intentionally NOT flushed
	// per batch: flushing them mid-project breaks cross-unit resolution
	// (SyntaxFlow `#->` over-resolves imported symbols — see TestImportClass).
	// They stay resident and are persisted by the final SaveToDatabase flush.
	//
	// NOTE: FlushCompileUnit is currently NOT called from the batch loop in
	// ssa_compile_fs.go because the per-batch instruction flush exposes two
	// deeper bugs (dbcache async-save channel lifetime → FeedBlock panic on
	// lazy reload; cross-unit store flush). It is retained here for re-enable
	// once those are fixed; shouldKeepCompileUnitBoundaryResident already
	// keeps BasicBlocks resident for that future path.
	c.diagnosticsTrack("ssa.ProgramCache.FlushCompileUnit",
		func() error {
			if c.instructions != nil {
				c.instructions.Flush()
			}
			return nil
		},
	)
	c.lastReleasedEditors = 0

	// Release program-level state for completed units (function bodies plus
	// program caches the flush path no longer needs).
	releasedFuncs := 0
	if c.program != nil {
		releasedFuncs = c.program.ReleaseCompletedUnitMemory(strings.Split(unitKey, ","))
	}

	// Single GC at unit-run end to reclaim the released resident memory.
	runtime.GC()

	if instructionCacheDebugEnabled() {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		log.Debugf("[ssa-ir-cache-flush] program=%s unit=%s mode=%s released_funcs=%d heap=%.1fMB resident=%d persisted=%d",
			c.program.GetProgramName(), unitKey, c.InstructionCacheMode(), releasedFuncs, float64(m.HeapInuse)/(1024*1024), c.CountInstruction(), c.InstructionPersistedCount())
	}
}

func (c *ProgramCache) CountReleasedEditors() int {
	if c == nil {
		return 0
	}
	return c.lastReleasedEditors
}

// FlushAuxSavers drains the auxiliary async/resident DB savers (index, offset,
// type). It does NOT spill instructions and does NOT clear any resident maps
// (index variable/member/class/consts, type resident), so it is safe to call
// between compile batches: cross-unit SyntaxFlow resolution keeps using the
// resident maps (TestImportClass), and BasicBlocks stay resident
// (TestPython_ImportWithInit, TestJsp_To_Java_Range). typeStore.flush marshals
// and persists resident types but leaves them resident so later-units / lazy
// builds / cross-unit queries still resolve. It exists to spread IrIndex/
// IrOffset/IrType writes across the whole compile instead of one giant final
// SaveToDatabase flush, which on a large project (javacms) backed up the async
// saver's FeedBlock and stalled the compile for >1h, and on javacms-core made
// the type-store flush (per-row UpsertIrType + json.Marshal) dominate the final
// flush CPU (~86%).
func (c *ProgramCache) FlushAuxSavers() {
	if c == nil || !c.HaveDatabaseBackend() {
		return
	}
	if c.indexes != nil {
		c.indexes.Flush() // indexStore.Flush -> indexSaver.Flush + offsetSaver.Flush
	}
	if c.types != nil {
		c.types.flush() // typeStore.flush: marshal+batch-INSERT resident types, keeps resident map
	}
}

// flushAuxStores clears only the non-instruction stores (types, sources) after
// a compile-unit flush. The instruction store is not touched: its
// compile-unit-split flush path already persisted ordinary instructions while
// keeping function/parameter/free-value boundary instructions resident for
// later cross-unit calls.
//
// Currently unused: FlushCompileUnit no longer clears aux stores (that broke
// cross-unit resolution). Retained for the future re-enable of full per-batch
// flush once the dbcache FeedBlock + cross-unit bugs are fixed.
func (c *ProgramCache) flushAuxStores() (cleared int) {
	if c == nil {
		return 0
	}

	if c.types != nil && c.types.resident != nil {
		c.types.resident = utils.NewSafeMapWithKey[int64, Type]()
		cleared += 100
	}
	if c.sources != nil {
		c.sources.mu.Lock()
		beforeSize := len(c.sources.payloads) + len(c.sources.editors)
		c.sources.payloads = make(map[string]*ssadb.IrSource)
		c.sources.persisted = make(map[string]struct{})
		c.sources.editors = make(map[string]*memedit.MemEditor)
		c.sources.editorsByURL = make(map[string]*memedit.MemEditor)
		c.sources.visitedURLs = make(map[string]*memedit.MemEditor)
		c.sources.mu.Unlock()
		cleared += beforeSize
	}
	return cleared
}

func (c *ProgramCache) CountInstruction() int {
	if c == nil || c.instructions == nil {
		return 0
	}
	return c.instructions.Count()
}

func (c *ProgramCache) CoolDownFunctionInstructions(function *Function) {
	if c == nil || c.instructions == nil || !c.HaveDatabaseBackend() || c.program == nil || c.program.DatabaseKind != ProgramCacheDBWrite {
		return
	}
	c.instructions.TrackFunctionFinish(function)
}

func (c *ProgramCache) rememberType(typ Type) {
	if c == nil || c.types == nil || utils.IsNil(typ) {
		return
	}
	c.types.remember(typ)
}

func (c *ProgramCache) getType(id int64) (Type, bool) {
	if c == nil || c.types == nil {
		return nil, false
	}
	return c.types.get(id)
}

func (c *ProgramCache) residentType(id int64) (Type, bool) {
	if c == nil || c.types == nil || c.types.resident == nil {
		return nil, false
	}
	return c.types.resident.Get(id)
}

func (c *ProgramCache) coolDownInstructions(ids []int64, ttl time.Duration) {
	if c == nil || c.instructions == nil {
		return
	}
	c.instructions.CoolDown(ids, ttl)
}

func (c *ProgramCache) deleteInstructionByID(id int64) {
	if c == nil || c.instructions == nil {
		return
	}
	c.instructions.Delete(id)
}

func (c *ProgramCache) residentInstructions() map[int64]Instruction {
	if c == nil || c.instructions == nil {
		return nil
	}
	return c.instructions.GetAllResident()
}

func (c *ProgramCache) hasResidentInstruction(id int64) bool {
	if id <= 0 {
		return false
	}
	_, ok := c.residentInstructions()[id]
	return ok
}

func (c *ProgramCache) findByVariableEx(mod ssadb.MatchMode, checkValue func(string) bool) []Instruction {
	if c == nil || c.indexes == nil {
		return nil
	}
	return c.indexes.FindByVariableEx(mod, checkValue, c.GetInstruction)
}

// setAtomicMaxIfGreater updates the atomic counter only when the new value is
// larger than the current one.
func setAtomicMaxIfGreater(counter *atomic.Int64, value int64) {
	if counter == nil {
		return
	}
	for {
		current := counter.Load()
		if value <= current {
			return
		}
		if counter.CAS(current, value) {
			return
		}
	}
}

func normalizeVariableName(name string) (normalized, member string) {
	if strings.HasPrefix(name, "#") {
		if _, memberName, ok := strings.Cut(name, "."); ok {
			member = memberName
		}
		if _, memberKey, ok := strings.Cut(name, "["); ok {
			member, _ = strings.CutSuffix(memberKey, "]")
		}
	}
	if len(name) > 1 {
		name = strings.TrimPrefix(name, "$")
	}
	return name, member
}
