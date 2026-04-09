package ssa

import (
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"go.uber.org/atomic"
)

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

	progressMu sync.RWMutex
	progressFn func(int)
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
		mode:       mode,
		program:    prog,
		db:         db,
		nextID:     atomic.NewInt64(0),
		progressFn: func(int) {},
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
			store.resident = utils.NewSafeMapWithKey[int64, Instruction]()
			store.flushResidentOnClose = true
			store.saveSize = min(max(saveSize*20, 5000), maxSaveSize)
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
				return shouldKeepInstructionResident(inst) || shouldDelayInstructionEviction(inst)
			}),
		)
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

func (s *instructionStore) Close(progress func(int)) {
	if s == nil {
		return
	}
	s.setProgress(progress)

	switch {
	case s.writer != nil:
		s.writer.Close()
	case s.flushResidentOnClose:
		s.flushResidentOnCloseOnly()
	}
}

func (s *instructionStore) flushResidentOnCloseOnly() {
	if s == nil || s.resident == nil || s.db == nil {
		return
	}

	saveBatch := saveInstructionIrCodesFast(s.program, s.db, s.notifyProgress)
	batch := make([]*ssadb.IrCode, 0, s.saveSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := saveBatch(batch); err != nil {
			log.Errorf("save ir code batch fast path failed: %v", err)
		}
		batch = make([]*ssadb.IrCode, 0, s.saveSize)
	}

	s.resident.ForEach(func(_ int64, inst Instruction) bool {
		irCode, err := marshalIrCode(inst)
		if err != nil {
			log.Errorf("marshal ir code failed: %v", err)
			return true
		}
		if irCode == nil {
			return true
		}
		batch = append(batch, irCode)
		if len(batch) >= s.saveSize {
			flush()
		}
		return true
	})
	flush()
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
	irCode, err := marshalIrCode(inst)
	if err != nil {
		return nil, err
	}

	updateExisting := false
	if lz, ok := ToLazyInstruction(inst); ok && lz != nil {
		// Dirty lazy instructions already have a DB row keyed by
		// (program_name, code_id), so eviction must update that row instead of
		// inserting a second copy.
		updateExisting = lz.ShouldSave()
	}

	if irCode == nil {
		if instructionCacheEventDebugEnabled() {
			log.Debugf("[ssa-ir-cache] save-skip: program=%s id=%d opcode=%s reason=%s",
				s.program.GetProgramName(),
				inst.GetId(),
				inst.GetOpcode().String(),
				evictionReasonName(reason),
			)
		}
		return nil, nil
	}

	return &instructionPersistRecord{
		IrCode:         irCode,
		Opcode:         inst.GetOpcode(),
		Reason:         reason,
		UpdateExisting: updateExisting,
		CodeID:         inst.GetId(),
	}, nil
}

func (s *instructionStore) saveInstructionPersistRecords(records []*instructionPersistRecord) error {
	if len(records) == 0 {
		return nil
	}

	start := time.Now()
	var saveErr error
	saveStep := func() error {
		saveErr = utils.GormTransaction(s.db, func(tx *gorm.DB) error {
			for _, record := range records {
				if record == nil || record.IrCode == nil {
					continue
				}
				if record.UpdateExisting {
					if err := ssadb.UpsertIrCode(tx, record.IrCode); err != nil {
						return err
					}
					continue
				}
				if err := tx.Save(record.IrCode).Error; err != nil {
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
		return saveErr
	}

	cost := time.Since(start)
	perItemCost := cost / time.Duration(len(records))
	if perItemCost <= 0 {
		perItemCost = cost
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

func saveInstructionIrCodesFast(prog *Program, db *gorm.DB, notify func(int)) dbcache.SaveFunc[*ssadb.IrCode] {
	return func(records []*ssadb.IrCode) error {
		if len(records) == 0 {
			return nil
		}
		var saveErr error
		saveStep := func() error {
			saveErr = utils.GormTransaction(db, func(tx *gorm.DB) error {
				for _, irCode := range records {
					if irCode == nil {
						continue
					}
					if err := tx.Save(irCode).Error; err != nil {
						return err
					}
				}
				return nil
			})
			return saveErr
		}
		if prog != nil {
			prog.DiagnosticsTrack("ssa.Database.SaveIrCodeBatchFastPath", saveStep)
		} else {
			saveStep()
		}
		if saveErr != nil {
			return saveErr
		}
		if notify != nil {
			notify(len(records))
		}
		return nil
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
		}
		if funcID <= 0 {
			if fn := inst.GetFunc(); fn != nil {
				funcID = fn.GetId()
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
	ret := ssadb.EmptyIrCode(inst.GetProgramName(), inst.GetId())
	if ok := marshalInstruction(inst, ret); !ok {
		return nil, nil
	}
	return ret, nil
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
