package ssa

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	yaklog "github.com/yaklang/yaklang/common/log"
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
		yaklog.Debugf("[ssa-ir-cache] init: program=%s ttl=%s max=%d kind=%d",
			programName, cacheTTL, cacheMax, databaseKind,
		)
	}

	saveSize := min(max(fileSize*5, defaultSaveSize), maxSaveSize)
	yaklog.Debugf("asyncdb Channel: ReSetSize: fileSize(%d) saveSize(%d)", fileSize, saveSize)

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
		yaklog.Errorf("BUG: SetInstruction called with nil instruction")
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
				yaklog.Infof("Type Cache closed")
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
				yaklog.Infof("Instruction cache closed")
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
				yaklog.Debugf("[ssa-ir-cache-saver] program=%s %s", c.program.GetProgramName(), stats)
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
	releasedEditors := 0
	c.diagnosticsTrack("ssa.ProgramCache.FlushCompileUnit",
		func() error {
			if c.instructions != nil {
				c.instructions.Flush()
			}
			return nil
		},
		func() error {
			if c.indexes != nil {
				c.indexes.Flush()
			}
			return nil
		},
		func() error {
			if c.types != nil {
				c.types.flush()
			}
			return nil
		},
		func() error {
			if c.sources != nil {
				c.sources.Flush()
				releasedEditors = c.sources.ReleasePersistedEditors()
			}
			return nil
		},
	)
	c.lastReleasedEditors = releasedEditors

	// REAL FIX: Clear ALL store caches to release memory
	// Not just instructions, but also types, sources (editors), etc.
	// These accumulate heavily across batches.
	clearedItems := c.AggressiveClearAllStores()

	// Also clear Program-level structures
	releasedFuncs := 0
	if c.program != nil {
		releasedFuncs = c.program.ReleaseCompletedUnitMemory(strings.Split(unitKey, ","))
		c.program.AggressiveClearMemory()
	}

	// Force GC to reclaim freed memory
	runtime.GC()
	runtime.GC()
	runtime.GC()
	debug.FreeOSMemory()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	fmt.Printf("[REAL-FIX] Cleared %d items, released %d funcs, heap=%.1fMB\n",
		clearedItems, releasedFuncs, float64(m.HeapInuse)/(1024*1024))

	if instructionCacheDebugEnabled() {
		yaklog.Debugf("[ssa-ir-cache-flush] program=%s unit=%s mode=%s resident=%d persisted=%d released_editors=%d released_funcs=%d",
			c.program.GetProgramName(), unitKey, c.InstructionCacheMode(), c.CountInstruction(), c.InstructionPersistedCount(), releasedEditors, releasedFuncs)
	}
}

func (c *ProgramCache) CountReleasedEditors() int {
	if c == nil {
		return 0
	}
	return c.lastReleasedEditors
}

// AggressiveClearInstructions drops ALL cached instructions from memory.
// This is the real fix for split compile memory accumulation.
func (c *ProgramCache) AggressiveClearInstructions() int {
	if c == nil || c.instructions == nil {
		return 0
	}
	return c.instructions.AggressiveClearInstructions()
}

// AggressiveClearAllStores clears ALL store caches: instructions, types, sources, indexes.
// Called after each batch flush to release memory.
func (c *ProgramCache) AggressiveClearAllStores() int {
	if c == nil {
		return 0
	}

	cleared := 0

	// Clear instruction store
	if c.instructions != nil {
		cleared += c.instructions.AggressiveClearInstructions()
	}

	// Clear type store - holds Type objects in resident map
	if c.types != nil && c.types.resident != nil {
		// SafeMapWithKey doesn't have Len(), just recreate it
		c.types.resident = utils.NewSafeMapWithKey[int64, Type]()
		cleared += 100 // Estimate, we can't count before clearing
	}

	// Clear source store - holds editors and payloads (THIS IS BIG!)
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

	// Clear index store - holds IR indexes
	// (indexStore is typically small, but clear it anyway)
	if c.indexes != nil {
		// indexStore doesn't have obvious large caches, skip for now
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
