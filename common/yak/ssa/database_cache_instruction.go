package ssa

import (
	"runtime"
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"go.uber.org/atomic"
)

// irCodePool reuses IrCode structs to reduce GC pressure during instruction
// marshal+save. Each IrCode has ~50 fields; allocating 5M of them at close
// time produces 2.2GB of short-lived allocations. The pool returns a fully
// zeroed IrCode; callers MUST NOT return an IrCode that is still referenced
// by a pending DB save.
var irCodePool = sync.Pool{
	New: func() interface{} {
		return new(ssadb.IrCode)
	},
}

// acquireIrCode gets a zeroed IrCode from the pool.
func acquireIrCode() *ssadb.IrCode {
	ic := irCodePool.Get().(*ssadb.IrCode)
	*ic = ssadb.IrCode{} // full zero — clears ALL fields
	return ic
}

// releaseIrCode returns an IrCode to the pool after it has been persisted.
// The caller MUST ensure the IrCode is no longer referenced by any DB
// transaction or save queue.
func releaseIrCode(ic *ssadb.IrCode) {
	if ic == nil {
		return
	}
	*ic = ssadb.IrCode{} // clear all fields before returning to pool
	irCodePool.Put(ic)
}

// instructionStore owns instruction residency for exactly one mode.
// Keeping the three concrete backends visible here is intentional: only one is
// active at a time, and a local interface layer would just mirror the same
// mode-specific operations without simplifying the control flow.
type instructionStore struct {
	mode ProgramCacheKind

	program *Program
	db      *gorm.DB

	nextID *atomic.Int64

	// resident is used by pure-memory mode and the DB-write fast path that keeps
	// everything resident until Close flushes the final snapshot.
	resident *utils.SafeMapWithKey[int64, Instruction]
	// reader is used by DB-read mode with lazy reload and bounded residency.
	reader *dbcache.ResidencyCacheWithKey[int64, Instruction]
	// writer is used by DB-write mode with async marshal + save.
	writer *dbcache.Cache[Instruction, *instructionPersistRecord]

	flushResidentOnClose bool
	saveSize             int
	// residentCache drives the adaptive DB-write fast path: all instructions
	// stay resident until Close, with synchronous incremental batched flush.
	residentCache *dbcache.ResidentFlushCache[int64, Instruction, *instructionPersistRecord]

	persistedCount   atomic.Int64
	batchSaveCount   atomic.Int64
	flushRequests    atomic.Int64
	persistEnqueued  atomic.Int64
	persistCompleted atomic.Int64
	writeOperations  atomic.Int64

	// persistedIDs tracks which instruction IDs have been successfully
	// persisted to the DB. This prevents duplicate INSERTs when the async
	// persist pipeline (MarkDirty+Barrier) or compile-unit split causes
	// the same instruction to be flushed multiple times.
	persistedIDsMu sync.Mutex
	persistedIDs   map[int64]struct{}

	// reservedIDs tracks IDs that have been enqueued for persistence
	// (MarkDirty called) but not yet saved. When Set is called for a
	// reserved ID, the existing pending item is updated in-place
	// (writer.Set replaces the value) instead of being skipped, so
	// legitimate content updates are not lost.
	reservedIDsMu sync.Mutex
	reservedIDs   map[int64]struct{}

	progressMu sync.RWMutex
	progressFn func(int)

	compileUnitSplit bool
}

// instructionPersistRecord is the persisted form of a single instruction save
// request, including the editor/source linkage needed by IrCode rows.
type instructionPersistRecord struct {
	IrCode         *ssadb.IrCode
	Opcode         Opcode
	Reason         utils.EvictionReason
	UpdateExisting bool
	CodeID         int64
}

func newInstructionStore(
	cfg *ssaconfig.Config,
	prog *Program,
	mode ProgramCacheKind,
	db *gorm.DB,
	saveSize int,
) *instructionStore {
	cfg = ensureProgramConfig(cfg)
	saveSize = min(max(saveSize, defaultSaveSize), maxSaveSize)

	store := &instructionStore{
		mode:         mode,
		program:      prog,
		db:           db,
		nextID:       atomic.NewInt64(0),
		progressFn:   func(int) {},
		persistedIDs: make(map[int64]struct{}),
		reservedIDs:  make(map[int64]struct{}),

		compileUnitSplit: cfg.GetCompileUnitSplit(),
	}

	switch mode {
	case ProgramCacheMemory:
		store.resident = utils.NewSafeMapWithKey[int64, Instruction]()
	case ProgramCacheDBRead:
		cacheTTL, cacheMax := resolveInstructionCacheSettings(cfg)
		var reader *dbcache.ResidencyCacheWithKey[int64, Instruction]
		reader = dbcache.NewResidencyCacheWithKey[int64, Instruction](
			cacheTTL,
			cacheMax,
			func(key int64, generation uint64, reason utils.EvictionReason) bool {
				reader.FinishPersist(key, generation, true)
				return true
			},
			store.loadInstruction,
			func(inst Instruction) bool {
				return shouldKeepInstructionResident(inst) || shouldDelayInstructionEviction(inst)
			},
		)
		store.reader = reader
	case ProgramCacheDBWrite:
		if useAdaptiveInstructionFastPath(cfg) {
			store.flushResidentOnClose = true
			store.saveSize = min(max(saveSize*20, 5000), maxSaveSize)
			store.residentCache = dbcache.NewResidentFlushCache[int64, Instruction, *instructionPersistRecord](
				store.saveSize,
				store.marshalResidentRecord,
				store.saveInstructionPersistRecords,
			)
			store.resident = store.residentCache.Map()
			return store
		}

		cacheTTL, cacheMax := resolveInstructionCacheSettings(cfg)
		instructionSaveSize, persistLimit := resolveInstructionPersistenceTuning(cfg, saveSize)
		store.writer = dbcache.NewCache[Instruction, *instructionPersistRecord](
			cacheTTL,
			cacheMax,
			store.marshalInstructionRecord,
			store.saveInstructionPersistRecords,
			store.loadInstruction,
			dbcache.WithContext(cfg.GetContext()),
			dbcache.WithSaveSize(instructionSaveSize),
			dbcache.WithPersistLimit(persistLimit),
			dbcache.WithSaveTimeout(saveTime),
			dbcache.WithName("Instruction"),
			dbcache.WithSkipEviction(func(inst Instruction) bool {
				return shouldKeepInstructionResident(inst) ||
					(store.compileUnitSplit && shouldKeepCompileUnitBoundaryResident(inst)) ||
					shouldDelayInstructionEviction(inst)
			}),
		)
		// Register settlement callback: unreserveID on ALL terminal paths
		// (success, failed, stale, rejected). On success, markPersisted
		// is also called (from saveInstructionPersistRecords), so the
		// settlement callback only needs to handle non-success unreserve.
		// On success, markPersisted already calls unreserveID.
		// On non-success, unreserveID must be called here to prevent
		// reserved IDs from leaking when items leave the pipeline
		// without going through saveFn (e.g. stale generation, marshal error).
		store.writer.SetSettlementCallback(func(key int64, generation uint64, outcome dbcache.PersistOutcome) {
			switch outcome {
			case dbcache.PersistSuccess:
				// markPersisted already called unreserveID in saveFn
			case dbcache.PersistFailed, dbcache.PersistStale, dbcache.PersistRejected, dbcache.PersistMarshalFailed:
				// Release reserved on non-success terminal paths
				store.unreserveID(key)
			}
		})
	}
	return store
}

func (s *instructionStore) Set(inst Instruction) {
	if s == nil || utils.IsNil(inst) {
		return
	}
	id := inst.GetId()
	if id <= 0 {
		id = s.nextID.Inc()
		inst.SetId(id)
	} else {
		setAtomicMaxIfGreater(s.nextID, id)
	}

	// If already persisted: skip — DB already has this row.
	if s.isPersisted(id) {
		return
	}
	// If reserved (in-flight save): update the resident item's value
	// WITHOUT clearing pending or incrementing generation. Regular
	// writer.Set would clear pending=false and generation++, causing
	// SnapshotForPersist to find a generation mismatch → FinishPersist(false)
	// → item not deleted → potential duplicate on re-flush.
	if s.isReserved(id) {
		switch {
		case s.writer != nil:
			s.writer.UpdateWhilePending(id, inst)
		case s.reader != nil:
			s.reader.UpdateWhilePending(id, inst)
		case s.resident != nil:
			s.resident.Set(id, inst) // memory mode: no pending concept
		}
		return
	}
	switch {
	case s.writer != nil:
		s.writer.Set(inst)
	case s.reader != nil:
		s.reader.Set(id, inst)
	case s.resident != nil:
		s.resident.Set(id, inst)
	}
}

func (s *instructionStore) Get(id int64) Instruction {
	if s == nil || id <= 0 {
		return nil
	}
	switch {
	case s.writer != nil:
		if inst, ok := s.writer.Get(id); ok {
			return inst
		}
	case s.reader != nil:
		if inst, ok := s.reader.Get(id); ok {
			return inst
		}
	case s.resident != nil:
		if inst, ok := s.resident.Get(id); ok {
			return inst
		}
	}
	return nil
}

func (s *instructionStore) Delete(id int64) {
	if s == nil || id <= 0 {
		return
	}
	switch {
	case s.writer != nil:
		s.writer.Delete(id)
	case s.reader != nil:
		s.reader.Delete(id)
	case s.resident != nil:
		s.resident.Delete(id)
	}
}

func (s *instructionStore) Count() int {
	if s == nil {
		return 0
	}
	switch {
	case s.writer != nil:
		return s.writer.Count()
	case s.reader != nil:
		return s.reader.Count()
	case s.resident != nil:
		return s.resident.Count()
	default:
		return 0
	}
}

func (s *instructionStore) CoolDown(ids []int64, ttl time.Duration) {
	if s == nil || len(ids) == 0 || ttl <= 0 {
		return
	}
	switch {
	case s.writer != nil:
		s.writer.CoolDown(ids, ttl)
	case s.reader != nil:
		s.reader.CoolDownKeys(ids, ttl)
	}
}

func (s *instructionStore) Track(ids []int64) {
	if s == nil || len(ids) == 0 {
		return
	}
	switch {
	case s.writer != nil:
		s.writer.Track(ids)
	case s.reader != nil:
		s.reader.TrackKeys(ids)
	}
}

func (s *instructionStore) TrackFunctionFinish(function *Function) {
	if s == nil || s.mode != ProgramCacheDBWrite || s.writer == nil {
		return
	}
	ids := collectFinishedFunctionInstructionIDs(function)
	if len(ids) == 0 {
		return
	}
	s.writer.Track(ids)
}

func (s *instructionStore) Flush(onComplete ...func()) {
	if s == nil || s.mode != ProgramCacheDBWrite {
		return
	}
	var callback func()
	if len(onComplete) > 0 {
		callback = onComplete[0]
	}
	s.flushRequests.Add(1)
	switch {
	case s.writer != nil:
		// Always keep boundary instructions (Function, Parameter, FreeValue,
		// BasicBlock, ParameterMember, SideEffect, ExternLib) resident for
		// cross-unit resolution, regardless of compileUnitSplit. The old
		// non-split path (writer.Flush) evicted these boundary instructions
		// mid-compile, breaking cross-unit resolution and dramatically
		// reducing total instruction count (e.g. Hadoop: 5.1M -> 1.94M).
		s.flushCompileUnitWriter(callback)
	case s.residentCache != nil:
		if err := s.residentCache.Flush(false); err != nil {
			log.Errorf("flush resident instructions failed: %v", err)
		} else if callback != nil {
			callback()
		}
	}
}

// FlushSaver drains the async save pipeline (marshal→saver) without
// evicting new items from the resident cache. This ensures all pending
// DB writes complete before another store (e.g. typeStore) starts writing,
// preventing concurrent SQLite writes that can cause index corruption
// with PRAGMA synchronous=OFF under memory pressure.
func (s *instructionStore) FlushSaver() {
	if s == nil || s.mode != ProgramCacheDBWrite {
		return
	}
	if s.writer != nil {
		// Barrier waits for all pending async writes to complete
		_ = s.writer.Barrier()
	}
}

func (s *instructionStore) flushCompileUnitWriter(onComplete func()) {
	if s == nil || s.writer == nil {
		return
	}
	// Incremental iteration via ForEach — no full map copy (GetAll).
	// On Hadoop with 5M instructions, GetAll allocates a 5M-entry map;
	// ForEach iterates without copying the underlying map.
	ids := make([]int64, 0, 1024)
	s.writer.ForEach(func(id int64, inst Instruction) bool {
		if shouldKeepCompileUnitBoundaryResident(inst) {
			return true // continue
		}
		// Skip IDs that have already been persisted (prevents duplicate INSERTs
		// when async persist pipeline or compile-unit split re-visits the same instruction)
		if s.isPersisted(id) {
			return true // continue
		}
		ids = append(ids, id)
		return true
	})
	// Mark dirty for async persistence with persistedIDs guard.
	// MarkDirty enqueues dirty keys (non-blocking, with dedup), Barrier
	// waits for writer completion. The persistedIDs guard prevents
	// duplicate INSERTs when Close() encounters items that were already
	// persisted by a previous MarkDirty+Barrier (e.g. re-added during
	// cross-unit resolution).
	// Use FlushKeys (synchronous, no persistLimit backpressure) instead of
	// MarkDirty+Barrier. MarkDirty's persistLimit backpressure (32768 for
	// large projects) causes PersistRejected for items above the limit —
	// those items stay in cache with pending=false, unsaved to DB. When
	// later compile units access them, resolveLinkedValue falls through to
	// lazy creation, which under pair-first member relations triggers O(N)
	// GetMembersByKeyString traversal. This was the root cause of chat.go
	// taking 26 minutes to compile (vs 38ms on main).
	//
	// FlushKeys calls QueueKeys (no persistLimit) + Wait (synchronous).
	// This matches main's behavior exactly. persistedIDs guard in
	// saveInstructionPersistRecords prevents duplicate INSERTs.
	for _, id := range ids {
		s.reserveID(id)
	}
	// Use MarkDirtyAsync (async, no backpressure) instead of FlushKeys (sync).
	// MarkDirtyAsync enqueues keys for background persistence and returns
	// immediately — the compile thread is never blocked by serialization or
	// DB writes. The saver goroutine drains the pipeline and evicts each
	// saved entry via FinishPersist -> delete(c.data, key), so memory is
	// released asynchronously while compilation continues.
	//
	// FlushKeys was synchronous (Wait for all saves to complete), blocking
	// compilation during DB writes. MarkDirty (with backpressure) also
	// blocked when PendingCount exceeded persistLimit. MarkDirtyAsync has
	// no such blocking — SaveToDatabase's Barrier drains the pipeline.
	s.writer.MarkDirtyAsync(ids, utils.EvictionReasonCapacityReached)
	// Asynchronously drain the persist pipeline (wait for saves to finish,
	// evict saved entries via FinishPersist -> delete) and shrink the map.
	// This runs in the background so compilation is never blocked, but memory
	// is actually reclaimed shortly after the flush returns. Without this,
	// entries stay resident (resident_after == resident_before) and memory
	// only drops at SaveToDatabase — the Hadoop 21GB peak.
	// Batch-scoped async drain: wait only for THIS flush's keys to settle so
	// the callback reports a truthful resident_after and the map shrinks as
	// soon as this unit's instructions are evicted, even when later units have
	// already enqueued more work.
	s.writer.AsyncDrainKeysAndShrink(ids, onComplete)
}

func (s *instructionStore) GetAllResident() map[int64]Instruction {
	if s == nil {
		return nil
	}
	switch {
	case s.writer != nil:
		return s.writer.GetAll()
	case s.reader != nil:
		return s.reader.GetAll()
	case s.resident != nil:
		return s.resident.GetAll()
	default:
		return nil
	}
}

func (s *instructionStore) DisableSpill() {
	if s == nil || s.writer == nil {
		return
	}
	s.writer.DisableSave()
}

func (s *instructionStore) EnableSpill() {
	if s == nil || s.writer == nil {
		return
	}
	s.writer.EnableSave()
}

func (s *instructionStore) IsSpillDisabled() bool {
	if s == nil || s.writer == nil {
		return false
	}
	return s.writer.IsSaveDisabled()
}

func (s *instructionStore) IsClosed() bool {
	if s == nil {
		return false
	}
	switch {
	case s.writer != nil:
		return s.writer.IsClosed()
	case s.reader != nil:
		return s.reader.IsClosed()
	default:
		return false
	}
}

// CloseWithoutSave releases the instruction writer's resident cache and
// async pipeline without persisting to DB. Safe to call after
// SaveToDatabase completed successfully.
func (s *instructionStore) CloseWithoutSave() {
	if s == nil {
		return
	}
	if s.writer != nil {
		s.writer.CloseWithoutSave()
	}
}

func (s *instructionStore) Stats() dbcache.CacheStats {
	if s == nil {
		return dbcache.CacheStats{}
	}
	switch {
	case s.writer != nil:
		return s.writer.Stats()
	case s.reader != nil:
		return dbcache.CacheStats{ResidentCount: s.reader.Count()}
	case s.resident != nil:
		return dbcache.CacheStats{ResidentCount: s.resident.Count()}
	default:
		return dbcache.CacheStats{}
	}
}

func (s *instructionStore) ModeName() string {
	if s == nil {
		return "none"
	}
	switch {
	case s.writer != nil:
		return "writer"
	case s.reader != nil:
		return "reader"
	case s.flushResidentOnClose:
		return "resident-fast-path"
	case s.resident != nil:
		return "resident"
	default:
		return "none"
	}
}

// reserveID marks an ID as enqueued for persistence (in-flight).
func (s *instructionStore) reserveID(id int64) {
	if s == nil || id <= 0 {
		return
	}
	s.reservedIDsMu.Lock()
	s.reservedIDs[id] = struct{}{}
	s.reservedIDsMu.Unlock()
}

// isReserved returns true if the ID is currently in-flight.
func (s *instructionStore) isReserved(id int64) bool {
	if s == nil || id <= 0 {
		return false
	}
	s.reservedIDsMu.Lock()
	_, ok := s.reservedIDs[id]
	s.reservedIDsMu.Unlock()
	return ok
}

// unreserveID removes an ID from the reserved set.
func (s *instructionStore) unreserveID(id int64) {
	if s == nil || id <= 0 {
		return
	}
	s.reservedIDsMu.Lock()
	delete(s.reservedIDs, id)
	s.reservedIDsMu.Unlock()
}

// markPersisted records that an instruction ID has been persisted to the DB.
// Thread-safe. Idempotent.
func (s *instructionStore) markPersisted(id int64) bool {
	if s == nil || id <= 0 {
		return false
	}
	s.persistedIDsMu.Lock()
	if s.persistedIDs == nil {
		s.persistedIDs = make(map[int64]struct{})
	}
	if _, exists := s.persistedIDs[id]; exists {
		s.persistedIDsMu.Unlock()
		s.unreserveID(id)
		return false
	}
	s.persistedIDs[id] = struct{}{}
	s.persistedIDsMu.Unlock()
	s.unreserveID(id)
	return true
}

// isPersisted returns true if the instruction ID has already been persisted.
func (s *instructionStore) isPersisted(id int64) bool {
	if s == nil || id <= 0 {
		return false
	}
	s.persistedIDsMu.Lock()
	_, ok := s.persistedIDs[id]
	s.persistedIDsMu.Unlock()
	return ok
}

// flushAllUnpersisted flushes ALL remaining resident instructions,
// including boundary instructions (Function, Parameter, BasicBlock, etc.)
// that flushCompileUnitWriter intentionally keeps resident during compile.
// This is the final close path — every instruction must be persisted.
// Still checks isPersisted to prevent duplicate INSERTs.
func (s *instructionStore) flushAllUnpersisted() {
	if s == nil || s.writer == nil {
		return
	}
	ids := make([]int64, 0, 1024)
	persistedStillResident := make([]int64, 0)
	s.writer.ForEach(func(id int64, inst Instruction) bool {
		if s.isPersisted(id) {
			// Already persisted but still in writer cache (re-added by Set
			// after FinishPersist deleted it). Delete it — no need to re-save.
			persistedStillResident = append(persistedStillResident, id)
			return true
		}
		ids = append(ids, id)
		return true
	})
	// Delete already-persisted items from writer cache
	for _, id := range persistedStillResident {
		s.writer.Delete(id)
	}
	if len(ids) == 0 {
		return
	}
	for _, id := range ids {
		s.reserveID(id)
	}
	s.writer.MarkDirty(ids, utils.EvictionReasonCapacityReached)
}

func (s *instructionStore) Close(progress func(int)) error {
	if s == nil {
		return nil
	}
	s.setProgress(progress)

	progName := ""
	if s.program != nil {
		progName = s.program.GetProgramName()
	}

	switch {
	case s.writer != nil:
		// Enter final-draining mode: bypass persistLimit so all remaining
		// instructions are accepted into the persist pipeline without
		// PersistRejected. Compile-time MarkDirty before this point still
		// respected the limit; only the final drain is exempt.
		s.writer.BeginFinalDrain()

		// Bounded loop: continue flushing until no dirty items remain,
		// a hard error occurs, or maxPasses is exhausted. This replaces
		// the old fixed-two-pass approach that could not handle large
		// resident counts (>persistLimit).
		const maxPasses = 16
		var lastRemaining int
		for pass := 0; pass < maxPasses; pass++ {
			s.flushRequests.Add(1)
			s.flushAllUnpersisted()
			if err := s.writer.Barrier(); err != nil {
				log.Errorf("[ssa-instruction-store] program=%s Close: Barrier failed on pass %d: %v", progName, pass+1, err)
				return err
			}
			remaining := s.writer.Count()
			log.Infof("[ssa-instruction-store] program=%s Close: writer mode, pass %d/%d, remaining=%d", progName, pass+1, maxPasses, remaining)
			if remaining == 0 {
				break
			}
			// Check for saver failure — no point retrying if saves are failing
			if s.writer.IsClosed() {
				break
			}
			// If remaining hasn't changed, items may be stuck (e.g. shouldKeepResident
			// preventing eviction). Delete already-persisted items manually.
			if remaining == lastRemaining {
				// No progress — try deleting persisted-but-still-resident items
				s.flushAllUnpersisted() // re-check and delete persisted items
				remaining = s.writer.Count()
				if remaining == lastRemaining {
					err := utils.Errorf("program=%s Close: %d instructions still resident after %d passes — no progress, data may be lost", progName, remaining, pass+1)
					log.Errorf("[ssa-instruction-store] %v", err)
					return err
				}
			}
			lastRemaining = remaining
		}
		remaining := s.writer.Count()
		if remaining > 0 {
			err := utils.Errorf("program=%s Close: %d instructions still resident after final drain — data may be lost", progName, remaining)
			log.Errorf("[ssa-instruction-store] %v", err)
			return err
		}
		// All instructions persisted — safe to close without save.
		s.writer.CloseWithoutSave()
		log.Infof("[ssa-instruction-store] program=%s Close: writer closed, persisted=%d", progName, s.persistedCount.Load())
	case s.residentCache != nil:
		log.Infof("[ssa-instruction-store] program=%s Close: residentCache mode, count=%d", progName, s.residentCache.Count())
		s.flushRequests.Add(1)
		// Flush(true) persists all remaining items but keeps them resident
		// so the Program stays queryable for compile+scan reuse. Do NOT
		// call Close() — that would Clear() the resident map and break
		// query rules that depend on resident instructions (Risk=0 regression).
		if err := s.residentCache.Flush(true); err != nil {
			return err
		}
	}
	return nil
}

// marshalResidentRecord is the marshal callback for the adaptive fast-path
// residentCache. It turns an instruction into a persist record for the close
// flush (EvictionReasonDeleted). ok=false means the item has no DB row to
// write (e.g. an empty instruction) and is silently skipped; err is a marshal
// failure to accumulate and return. updateExisting is the cache's per-Flush
// decision: true on a close flush that follows an incremental flush (upsert),
// false otherwise (insert).
func (s *instructionStore) marshalResidentRecord(inst Instruction, updateExisting bool) (*instructionPersistRecord, bool, error) {
	irCode, err := s.marshalIrCodeWithReason(inst, utils.EvictionReasonDeleted)
	if err != nil {
		log.Errorf("marshal ir code failed: %v", err)
		return nil, false, err
	}
	if irCode == nil {
		// marshalIrCodeWithReason already released the pool IrCode
		return nil, false, nil
	}
	return &instructionPersistRecord{
		IrCode:         irCode,
		Opcode:         inst.GetOpcode(),
		Reason:         utils.EvictionReasonDeleted,
		UpdateExisting: updateExisting,
		CodeID:         inst.GetId(),
	}, true, nil
}

func (s *instructionStore) PreloadByIDsFast(ids []int64) {
	if s == nil || s.mode != ProgramCacheDBRead || s.program == nil || s.reader == nil || len(ids) == 0 {
		return
	}
	ssadb.PreloadIrCodesByIdsFast(ssadb.GetDB(), s.program.Name, ids)
	cache := ssadb.GetIrCodeCache(s.program.Name)
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := s.reader.GetResident(id); ok {
			continue
		}
		if ir, ok := cache.Get(id); ok {
			if inst, err := NewLazyInstructionFromIrCode(ir, s.program); err == nil {
				s.Set(inst)
			}
		}
	}
}

func (s *instructionStore) loadInstruction(id int64) (Instruction, error) {
	start := time.Now()
	inst, err := NewLazyInstruction(s.program, id)
	if err != nil {
		return nil, err
	}
	if instructionCacheEventDebugEnabled() {
		log.Debugf("[ssa-ir-cache] reload: program=%s id=%d opcode=%s cost=%s",
			s.program.GetProgramName(), id, inst.GetOpcode().String(), time.Since(start),
		)
	}
	if instructionReloadStackDebugEnabled() {
		log.Warnf("[ssa-ir-cache-reload] program=%s id=%d opcode=%s cost=%s",
			s.program.GetProgramName(), id, inst.GetOpcode().String(), time.Since(start),
		)
		utils.PrintCurrentGoroutineRuntimeStack()
	}
	return inst, nil
}

func (s *instructionStore) marshalInstructionRecord(inst Instruction, reason utils.EvictionReason) (*instructionPersistRecord, error) {
	irCode, err := s.marshalIrCodeWithReason(inst, reason)
	if err != nil {
		return nil, err
	}
	if irCode == nil {
		return nil, nil
	}

	updateExisting := false
	if lz, ok := ToLazyInstruction(inst); ok && lz != nil {
		// Dirty lazy instructions already have a DB row keyed by
		// (program_name, code_id), so eviction must update that row instead of
		// inserting a second copy.
		updateExisting = lz.ShouldSave()
	}
	updateExisting = updateExisting || s.isPersisted(inst.GetId())

	return &instructionPersistRecord{
		IrCode:         irCode,
		Opcode:         inst.GetOpcode(),
		Reason:         reason,
		UpdateExisting: updateExisting,
		CodeID:         inst.GetId(),
	}, nil
}

func errCloseFlushNotPersisted(inst Instruction) error {
	return utils.Errorf("close flush: instruction id=%d opcode=%s not marshaled and not found in database",
		inst.GetId(), inst.GetOpcode())
}

func (s *instructionStore) instructionExistsInDB(id int64) bool {
	if s == nil || s.db == nil || s.program == nil || id <= 0 {
		return false
	}
	return ssadb.ExistsIrCodeById(s.db, s.program.Name, id)
}

func (s *instructionStore) acknowledgeCloseFlushIfPersisted(inst Instruction) bool {
	if !s.instructionExistsInDB(inst.GetId()) {
		return false
	}
	s.notifyProgress(1)
	return true
}

func (s *instructionStore) expandRelationPersistRecords(records []*instructionPersistRecord) []*instructionPersistRecord {
	if s == nil || s.program == nil || s.program.Cache == nil || len(records) == 0 {
		return records
	}
	expanded := make([]*instructionPersistRecord, 0, len(records)*2)
	queued := make(map[int64]struct{}, len(records)*2)
	appendRecord := func(record *instructionPersistRecord) {
		if record == nil || record.IrCode == nil {
			return
		}
		if _, ok := queued[record.CodeID]; ok {
			return
		}
		queued[record.CodeID] = struct{}{}
		expanded = append(expanded, record)
	}
	var walk func(record *instructionPersistRecord)
	walk = func(record *instructionPersistRecord) {
		appendRecord(record)
		if record == nil || record.IrCode == nil {
			return
		}
		for _, id := range relationInstructionIDs(record.IrCode) {
			if _, ok := queued[id]; ok {
				continue
			}
			linked, ok := s.writer.GetResident(id)
			if !ok || linked == nil {
				continue
			}
			linkedIr, err := marshalIrCode(linked)
			if err != nil || linkedIr == nil {
				continue
			}
			if progName := applicationProgramName(s.program); progName != "" {
				linkedIr.ProgramName = progName
			}
			relRecord := &instructionPersistRecord{
				IrCode: linkedIr,
				Opcode: linked.GetOpcode(),
				Reason: record.Reason,
				CodeID: linked.GetId(),
			}
			walk(relRecord)
		}
	}
	for _, record := range records {
		walk(record)
	}
	return expanded
}

func (s *instructionStore) saveInstructionPersistRecords(records []*instructionPersistRecord) error {
	if len(records) == 0 {
		return nil
	}
	records = s.expandRelationPersistRecords(records)
	// Filter out already-persisted INSERT-path records after expansion.
	// The expansion may pull in relation instructions that were already
	// saved in a prior batch — without this filter, the UNIQUE constraint
	// on ir_codes (program_name, code_id) would reject the duplicate INSERT
	// and fail the entire batch.
	// UPSERT-path records (UpdateExisting=true) are kept because they
	// intentionally update existing rows (e.g. SetExtern after flush).
	filtered := records[:0]
	for _, record := range records {
		if record == nil || record.IrCode == nil {
			continue
		}
		if record.UpdateExisting {
			filtered = append(filtered, record)
		} else if !s.isPersisted(record.CodeID) {
			filtered = append(filtered, record)
		}
	}
	records = filtered
	if len(records) == 0 {
		return nil
	}
	recordCount := 0
	for _, record := range records {
		if record != nil && record.IrCode != nil {
			recordCount++
		}
	}
	s.persistEnqueued.Add(int64(recordCount))

	start := time.Now()
	var saveErr error
	saveStep := func() error {
		saveErr = utils.GormTransaction(s.db, func(tx *gorm.DB) error {
			// Separate insert and upsert paths:
			// - Insert path: batch CreateInBatches for speed
			// - Upsert path: SaveIrCodeBatch (delete+bulk insert) — avoids
			//   per-row FirstOrCreate SELECT that can miss the (program_name,
			//   code_id) index on large Postgres IR tables.
			var insertBatch []*ssadb.IrCode
			var upsertBatch []*ssadb.IrCode
			for _, record := range records {
				if record == nil || record.IrCode == nil {
					continue
				}
				if record.UpdateExisting {
					upsertBatch = append(upsertBatch, record.IrCode)
					continue
				}
				insertBatch = append(insertBatch, record.IrCode)
			}
			if len(upsertBatch) > 0 {
				if err := ssadb.SaveIrCodeBatch(tx, upsertBatch); err != nil {
					return err
				}
			}
			if len(insertBatch) > 0 {
				batchSize := saveIrCodeInsertBatchSize
				if len(insertBatch) < batchSize {
					batchSize = len(insertBatch)
				}
				if err := tx.CreateInBatches(insertBatch, batchSize).Error; err != nil {
					return err
				}
			}
			return nil
		})
		return saveErr
	}
	if s.program != nil {
		s.program.DiagnosticsTrack("ssa.Database.SaveIrCodeBatch", saveStep)
	} else {
		saveStep()
	}
	if saveErr != nil {
		// On save failure, unreserve all IDs in this batch so they can
		// be retried on the next flush attempt.
		for _, record := range records {
			if record != nil {
				s.unreserveID(record.CodeID)
			}
		}
		return saveErr
	}
	s.persistCompleted.Add(int64(recordCount))
	s.writeOperations.Add(int64(recordCount))

	// Release IrCode structs back to the pool after successful save.
	// This is safe because GormTransaction has completed — the DB now
	// holds its own copy of the data, and we no longer need the struct.
	for _, record := range records {
		if record != nil && record.IrCode != nil {
			releaseIrCode(record.IrCode)
		}
	}

	uniquePersisted := int64(0)
	// Track persisted IDs to prevent duplicate INSERTs and count each ID once.
	for _, record := range records {
		if record != nil && record.IrCode != nil && s.markPersisted(record.CodeID) {
			uniquePersisted++
		}
	}
	s.persistedCount.Add(uniquePersisted)
	batchNum := s.batchSaveCount.Add(1)
	cost := time.Since(start)
	perItemCost := cost / time.Duration(len(records))
	if perItemCost <= 0 {
		perItemCost = cost
	}
	// Log progress every 100 batches or if batch is large
	if batchNum%100 == 0 || len(records) > 1000 {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		progName := ""
		if s.program != nil {
			progName = s.program.GetProgramName()
		}
		log.Infof("[ssa-instruction-save] program=%s batch=%d records=%d persisted=%d cost=%v heapAlloc=%.1fMB heapObjects=%d",
			progName, batchNum, len(records), s.persistedCount.Load(), cost, float64(ms.Alloc)/1024/1024, ms.HeapObjects)
	}

	s.notifyProgress(len(records))
	if instructionCacheEventDebugEnabled() {
		programName := ""
		if s.program != nil {
			programName = s.program.GetProgramName()
		}
		for _, record := range records {
			if record == nil {
				continue
			}
			action := "save"
			if record.UpdateExisting {
				action = "upsert"
			}
			log.Debugf("[ssa-ir-cache] %s: program=%s id=%d opcode=%s reason=%s cost=%s",
				action, programName, record.CodeID, record.Opcode.String(), evictionReasonName(record.Reason), perItemCost,
			)
		}
	}
	return nil
}

func (s *instructionStore) setProgress(fn func(int)) {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if fn == nil {
		s.progressFn = func(int) {}
		return
	}
	s.progressFn = fn
}

func (s *instructionStore) notifyProgress(size int) {
	s.progressMu.RLock()
	fn := s.progressFn
	s.progressMu.RUnlock()
	if fn != nil {
		fn(size)
	}
}

func instructionLocationIDs(inst Instruction) (funcID, blockID int64) {
	if utils.IsNil(inst) {
		return 0, 0
	}
	if inst.GetOpcode() == SSAOpcodeBasicBlock && inst.GetId() > 0 {
		if lz, ok := ToLazyInstruction(inst); ok && lz != nil && lz.ir != nil {
			return lz.ir.CurrentFunction, inst.GetId()
		}
		if inner := inst.getAnInstruction(); inner != nil {
			funcID = inner.funcId
			if funcID <= 0 && inner.fun != nil {
				funcID = inner.fun.GetId()
			}
		}
		return funcID, inst.GetId()
	}
	if lz, ok := ToLazyInstruction(inst); ok && lz != nil && lz.ir != nil {
		return lz.ir.CurrentFunction, lz.ir.CurrentBlock
	}
	if inner := inst.getAnInstruction(); inner != nil {
		return inner.funcId, inner.blockId
	}
	return 0, 0
}

func marshalIrCode(inst Instruction) (*ssadb.IrCode, error) {
	ret := acquireIrCode()
	ret.ProgramName = inst.GetProgramName()
	ret.CodeID = inst.GetId()
	if !marshalInstruction(inst, ret, 0) {
		releaseIrCode(ret)
		return nil, nil
	}
	return ret, nil
}

func (s *instructionStore) marshalIrCodeWithReason(inst Instruction, reason utils.EvictionReason) (*ssadb.IrCode, error) {
	ret := acquireIrCode()
	ret.ProgramName = inst.GetProgramName()
	ret.CodeID = inst.GetId()
	if marshalInstruction(inst, ret, reason) {
		return ret, nil
	}

	if reason == utils.EvictionReasonDeleted {
		if s.acknowledgeCloseFlushIfPersisted(inst) {
			return nil, nil
		}
		return nil, errCloseFlushNotPersisted(inst)
	}
	return nil, nil
}

func evictionReasonName(reason utils.EvictionReason) string {
	switch reason {
	case utils.EvictionReasonDeleted:
		return "deleted"
	case utils.EvictionReasonCapacityReached:
		return "capacity"
	case utils.EvictionReasonExpired:
		return "expired"
	default:
		return "unknown"
	}
}

func (s *instructionStore) PersistedCount() int64 {
	if s == nil {
		return 0
	}
	return s.persistedCount.Load()
}

type InstructionPersistStats struct {
	Requests        int64
	Enqueued        int64
	Completed       int64
	WriteOperations int64
	UniquePersisted int64
	Resident        int64
	RemainingDirty  int64
	Pending         int64
	BatchCount      int64
}

func (s *instructionStore) PersistenceStats() InstructionPersistStats {
	if s == nil {
		return InstructionPersistStats{}
	}
	stats := InstructionPersistStats{
		Requests:        s.flushRequests.Load(),
		Enqueued:        s.persistEnqueued.Load(),
		Completed:       s.persistCompleted.Load(),
		WriteOperations: s.writeOperations.Load(),
		UniquePersisted: s.persistedCount.Load(),
		BatchCount:      s.batchSaveCount.Load(),
	}
	switch {
	case s.writer != nil:
		cacheStats := s.writer.Stats()
		stats.Resident = int64(s.writer.Count())
		stats.Pending = cacheStats.Saver.Pending
	case s.residentCache != nil:
		stats.Resident = int64(s.residentCache.Count())
	case s.reader != nil:
		stats.Resident = int64(s.reader.Count())
	case s.resident != nil:
		stats.Resident = int64(s.resident.Count())
	}
	// RemainingDirty is the count of resident items that have NOT been
	// persisted yet. It is derived as max(0, Resident - UniquePersisted).
	// This separates the unsaved dirty count from the live resident object
	// count: after SaveToDatabase, resident > 0 (for query reuse) but
	// remaining_dirty == 0 (all were persisted).
	stats.RemainingDirty = stats.Resident - stats.UniquePersisted
	if stats.RemainingDirty < 0 {
		stats.RemainingDirty = 0
	}
	return stats
}

func (c *ProgramCache) InstructionPersistedCount() int {
	if c == nil {
		return 0
	}
	if c.instructions == nil {
		return int(c.cleanedPersistedCount)
	}
	return int(c.instructions.PersistedCount())
}

func (c *ProgramCache) InstructionCacheMode() string {
	if c == nil || c.instructions == nil {
		return "none"
	}
	return c.instructions.ModeName()
}
