package ssa

import (
	"runtime"
	"strings"
	"time"

	"github.com/yaklang/gorm"
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
	lastReleasedEditors   int
	cleanedPersistedCount int64

	// cleaned is set by CleanBaseline to indicate the cache has been
	// fully released and no compilation state remains.
	cleaned atomic.Bool
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

// CloseWithoutSave releases all in-memory cache objects (instructions, types,
// indexes, sources) without persisting to DB. Safe to call after SaveToDatabase
// completed successfully. After this, the Program can still serve SyntaxFlow
// queries via lazy DB reads (GetInstruction falls back to DB when the resident
// cache is empty).
func (c *ProgramCache) CloseWithoutSave() {
	if c == nil {
		return
	}
	// Close the instruction writer's cache (releases resident instruction map,
	// marshal pipeline, saver goroutines). This is the largest memory consumer.
	if c.instructions != nil {
		c.instructions.CloseWithoutSave()
	}
	// Type/index/source stores don't have CloseWithoutSave, but nil-ing
	// the references allows GC to reclaim the resident maps. This is safe
	// because SaveToDatabase already persisted all data to the DB.
	c.types = nil
	c.indexes = nil
	c.sources = nil
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

	// heapSnapshot logs current heap usage. On large projects the instruction
	// Close-flush can balloon memory because all resident instructions are
	// marshaled into IrCode structs; periodic snapshots help correlate OOM with
	// the exact phase.
	heapSnapshot := func(label string) {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		log.Infof("[ssa-ir-cache-save] %s: heapAlloc=%.1fMB heapSys=%.1fMB heapObjects=%d numGC=%d",
			label, float64(ms.Alloc)/1024/1024, float64(ms.Sys)/1024/1024, ms.HeapObjects, ms.NumGC)
	}

	progName := ""
	if c.program != nil {
		progName = c.program.GetProgramName()
	}
	log.Infof("[ssa-ir-cache-save] program=%s SaveToDatabase started", progName)
	// Final barrier enter log (v3 §A.5)
	enterRemaining := int64(c.CountInstruction())
	enterPersisted := int64(c.InstructionPersistedCount())
	enterTotal := enterRemaining + enterPersisted
	log.Infof("[ssa-persist-final-barrier] event=enter program=%s instructions_total=%d already_persisted=%d remaining_dirty=%d", progName, enterTotal, enterPersisted, enterRemaining)
	heapSnapshot("SaveToDatabase.enter")

	steps := []func() error{
		// step0: flush the instruction async saver to ensure all pending
		// instruction DB writes complete before types/indexes start writing.
		// This prevents concurrent SQLite writes that caused "database disk
		// image is malformed" corruption with synchronous=OFF under memory
		// pressure (root cause of Hadoop scan failure, 2026-07-29).
		func() error {
			if c.instructions != nil {
				log.Infof("[ssa-ir-cache-save] program=%s step0: flushing instruction saver before type store close", progName)
				c.FlushInstructionSaver()
				log.Infof("[ssa-ir-cache-save] program=%s step0: instruction saver flushed", progName)
			}
			return nil
		},
		func() error {
			if c.types != nil {
				log.Infof("[ssa-ir-cache-save] program=%s step1: closing type store", progName)
				heapSnapshot("step1_types.close.enter")
				start := time.Now()
				if err := c.types.close(); err != nil {
					return err
				}
				log.Infof("[ssa-ir-cache-save] program=%s Type Cache closed, cost=%v", progName, time.Since(start))
				heapSnapshot("step1_types.close.exit")
			}
			return nil
		},
		func() error {
			if c.indexes != nil {
				log.Infof("[ssa-ir-cache-save] program=%s step2: closing index/offset store", progName)
				heapSnapshot("step2_indexes.close.enter")
				start := time.Now()
				if err := c.indexes.Close(); err != nil {
					return err
				}
				log.Infof("[ssa-ir-cache-save] program=%s Index store closed, cost=%v", progName, time.Since(start))
				heapSnapshot("step2_indexes.close.exit")
			}
			return nil
		},
		func() error {
			if c.instructions != nil {
				remaining := c.CountInstruction()
				persisted := c.InstructionPersistedCount()
				log.Infof("[ssa-ir-cache-save] program=%s step3: closing instruction store (resident=%d persisted=%d total=%d)",
					progName, remaining, persisted, remaining+persisted)
				heapSnapshot("step3_instructions.close.enter")
				start := time.Now()
				if err := c.instructions.Close(progress); err != nil {
					log.Errorf("[ssa-ir-cache-save] program=%s Instruction cache close FAILED: %v (cost=%v)", progName, err, time.Since(start))
					return err
				}
				log.Infof("[ssa-ir-cache-save] program=%s Instruction cache closed, cost=%v", progName, time.Since(start))
				heapSnapshot("step3_instructions.close.exit")
			}
			return nil
		},
		func() error {
			if c.sources != nil {
				log.Infof("[ssa-ir-cache-save] program=%s step4: closing source store", progName)
				heapSnapshot("step4_sources.close.enter")
				start := time.Now()
				if err := c.sources.Close(); err != nil {
					return err
				}
				log.Infof("[ssa-ir-cache-save] program=%s Source store closed, cost=%v", progName, time.Since(start))
				heapSnapshot("step4_sources.close.exit")
			}
			return nil
		},
		func() error {
			if c.program != nil && c.instructions != nil {
				persistStats := c.instructions.PersistenceStats()
				progName := c.program.GetProgramName()
				avgBatch := float64(0)
				if persistStats.BatchCount > 0 {
					avgBatch = float64(persistStats.WriteOperations) / float64(persistStats.BatchCount)
				}
				// Writer summary (v3 §A.3)
				log.Infof("[ssa-persist-writer-summary] program=%s request=%d enqueued=%d completed=%d write_ops=%d unique_persisted=%d resident=%d pending=%d persisted_instructions=%d batch_count=%d avg_batch=%.2f queue_depth_current=%d pending_current=%d errors=0",
					progName, persistStats.Requests, persistStats.Enqueued, persistStats.Completed,
					persistStats.WriteOperations, persistStats.UniquePersisted, persistStats.Resident,
					persistStats.Pending, persistStats.WriteOperations, persistStats.BatchCount, avgBatch,
					persistStats.Resident, persistStats.Pending)
			}
			return nil
		},
	}
	err := c.diagnosticsTrackErr("ssa.ProgramCache.SaveToDatabase", steps...)

	// Final barrier log (v3 §A.5): mid_flush_coverage and final_pressure_reduction
	finalStats := InstructionPersistStats{}
	if c.instructions != nil {
		finalStats = c.instructions.PersistenceStats()
	}
	finalRemaining := finalStats.RemainingDirty
	finalPersisted := finalStats.UniquePersisted
	total := finalRemaining + finalPersisted
	var midFlushCoverage, finalPressureReduction float64
	if total > 0 {
		midFlushCoverage = float64(finalPersisted) / float64(total)
		finalPressureReduction = 1.0 - float64(finalRemaining)/float64(total)
	}
	// Source/type/index remaining and saved
	var sourceRemaining, sourceSaved int64
	var typeRemaining, typeSaved int64
	var indexRemaining, indexSaved int64
	if c.sources != nil {
		sourceSaved = int64(c.sources.PersistedCount())
	}
	// typeStore and indexStore don't expose persistedCount in a simple way;
	// use 0 for remaining (they're flushed during SaveToDatabase steps)
	log.Infof("[ssa-persist-final-barrier] event=done program=%s instructions_total=%d already_persisted=%d remaining_dirty=%d request=%d enqueued=%d completed=%d write_ops=%d unique_persisted=%d resident=%d pending=%d mid_flush_coverage=%.4f final_pressure_reduction=%.4f source_remaining=%d source_saved=%d type_remaining=%d type_saved=%d index_remaining=%d index_saved=%d err=%v",
		progName, total, finalPersisted, finalRemaining, finalStats.Requests, finalStats.Enqueued,
		finalStats.Completed, finalStats.WriteOperations,
		finalStats.UniquePersisted, finalStats.Resident, finalStats.Pending,
		midFlushCoverage, finalPressureReduction,
		sourceRemaining, sourceSaved, typeRemaining, typeSaved, indexRemaining, indexSaved, err)
	log.Infof("[ssa-ir-cache-save] program=%s SaveToDatabase finished, err=%v", progName, err)
	return err
}

func (c *ProgramCache) FlushCompileUnit(unitKey string) {
	if c == nil || !c.HaveDatabaseBackend() {
		return
	}
	unitSample := unitKey
	if len(unitSample) > 80 {
		unitSample = unitSample[:77] + "..."
	}
	residentBefore := c.CountInstruction()
	persistedBefore := c.InstructionPersistedCount()
	var onComplete func()
	if instructionCacheDebugEnabled() {
		onComplete = func() {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			pendingAfter := int64(0)
			if c.instructions != nil {
				pendingAfter = c.instructions.PersistenceStats().Pending
			}
			log.Debugf("[ssa-persist-flush] event=completed reason=unit unit_key=%s persisted_after=%d resident_after=%d heap_after=%.1fMB pending_after=%d",
				unitSample, c.InstructionPersistedCount(), c.CountInstruction(),
				float64(m.HeapInuse)/(1024*1024), pendingAfter)
		}
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		log.Debugf("[ssa-persist-flush] event=request reason=unit unit_key=%s mode=%s heap_before=%.1fMB resident_before=%d persisted_before=%d",
			unitSample, c.InstructionCacheMode(), float64(m.HeapInuse)/(1024*1024), residentBefore, persistedBefore)
	}
	// Per-batch flush bounds memory by spilling ORDINARY instructions to DB
	// (flushCompileUnitWriter keeps BasicBlocks + Function/Parameter/FreeValue
	// boundary instructions resident, so block ScopeTable survives for lazy
	// builds). Stores (indexes/types/sources) are intentionally NOT flushed
	// per batch: flushing them mid-project breaks cross-unit resolution
	// (SyntaxFlow `#->` over-resolves imported symbols — see TestImportClass).
	// They stay resident and are persisted by the final SaveToDatabase flush.
	//
	// The instruction saver is asynchronous: this method only enqueues the
	// selected instructions. The completion callback above is the only place
	// that reports post-persist residency and heap values.
	c.diagnosticsTrack("ssa.ProgramCache.FlushCompileUnit",
		func() error {
			if c.instructions != nil {
				c.instructions.Flush(onComplete)
			}
			return nil
		},
	)
	if instructionCacheDebugEnabled() && c.instructions != nil {
		stats := c.instructions.PersistenceStats()
		log.Debugf("[ssa-persist-flush] event=enqueued reason=unit unit_key=%s mode=%s persisted=%d resident=%d pending=%d",
			unitSample, c.InstructionCacheMode(), c.InstructionPersistedCount(), c.CountInstruction(), stats.Pending)
	}
	c.lastReleasedEditors = 0

	// Release program-level state for completed units (function bodies plus
	// program caches the flush path no longer needs).
	if c.program != nil {
		c.program.ReleaseCompletedUnitMemory(strings.Split(unitKey, ","))
	}
}

// FlushInstructionSaver drains the instruction async saver's pending
// writes without evicting new items. Call this before flushing other stores
// (types, indexes) to prevent concurrent SQLite writes.
func (c *ProgramCache) FlushInstructionSaver() {
	if c == nil || c.instructions == nil {
		return
	}
	c.instructions.FlushSaver()
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
		if err := c.indexes.Flush(); err != nil {
			log.Errorf("FlushAuxSavers: index store flush failed: %v", err)
		}
	}
	if c.types != nil {
		if err := c.types.flush(); err != nil {
			log.Errorf("FlushAuxSavers: type store flush failed: %v", err)
		}
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

// FlushAccounting is a snapshot of the instruction persist accounting,
// used for observability and test verification.
type FlushAccounting struct {
	InstructionsTotal      int64
	AlreadyPersisted       int64
	RemainingDirty         int64
	RemainingPending       int64
	WriteOperations        int64
	UniquePersisted        int64
	Resident               int64
	Pending                int64
	MidFlushCoverage       float64
	FinalPressureReduction float64
}

// GetFlushAccounting returns flush accounting metrics after SaveToDatabase.
// Returns the final accounting snapshot: total = already_persisted + remaining_dirty.
// It reports unique persisted instruction IDs separately from write operations.
func (c *ProgramCache) GetFlushAccounting() *FlushAccounting {
	if c == nil {
		return nil
	}
	persistStats := InstructionPersistStats{}
	if c.instructions != nil {
		persistStats = c.instructions.PersistenceStats()
	}
	remaining := persistStats.RemainingDirty
	persisted := persistStats.UniquePersisted
	total := remaining + persisted

	var midFlushCoverage float64
	var finalPressureReduction float64
	if total > 0 {
		midFlushCoverage = float64(persisted) / float64(total)
		finalPressureReduction = 1.0 - float64(remaining)/float64(total)
	}

	return &FlushAccounting{
		InstructionsTotal:      total,
		AlreadyPersisted:       persisted,
		RemainingDirty:         remaining,
		RemainingPending:       persistStats.Pending,
		WriteOperations:        persistStats.WriteOperations,
		UniquePersisted:        persistStats.UniquePersisted,
		Resident:               persistStats.Resident,
		Pending:                persistStats.Pending,
		MidFlushCoverage:       midFlushCoverage,
		FinalPressureReduction: finalPressureReduction,
	}
}

// CleanBaseline releases ALL compilation state after SaveToDatabase.
// This is the v3 step E "clean baseline" — no compilation state retained.
//
// After CleanBaseline:
// - All writers/pending channels are closed
// - AST/token/parser, Function/BasicBlock/Value/Instruction graphs are nil'd
// - Variable/Scope/VersionedTable, types/index/source resident maps are nil'd
// - Diagnostics, recorder, callbacks, builders are nil'd
// - The Program is removed from ssaapi.ProgramCache
// - runtime.GC() is called to reclaim released memory
//
// The caller must call this only after SaveToDatabase has succeeded.
// After CleanBaseline, the cache cannot be used for further writes.
func (c *ProgramCache) CleanBaseline() {
	if c == nil {
		return
	}

	// Save persisted count before nil'ing (for post-cleanup verification)
	if c.instructions != nil {
		c.cleanedPersistedCount = int64(c.InstructionPersistedCount())
	}

	// Close instruction store if not already closed
	if c.instructions != nil && !c.instructions.IsClosed() {
		c.instructions.CloseWithoutSave()
	}

	// Nil out all stores — allows GC to reclaim resident maps
	c.instructions = nil
	c.types = nil
	c.indexes = nil
	c.sources = nil

	// Nil program reference (breaks reference cycle)
	c.program = nil

	// Mark as cleaned
	c.cleaned.Store(true)

	// Force GC to reclaim released memory
	runtime.GC()
}

// IsCleaned returns true if CleanBaseline has been called.
func (c *ProgramCache) IsCleaned() bool {
	if c == nil {
		return false
	}
	return c.cleaned.Load()
}
