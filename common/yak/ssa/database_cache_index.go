package ssa

import (
	"github.com/yaklang/gorm"
	"strconv"

	"github.com/yaklang/yaklang/common/utils"
	"sync"

	"github.com/yaklang/yaklang/common/utils/dbcache"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	IndexSaveSize = 2000
)

type indexStore struct {
	mode    ProgramCacheKind
	program *Program
	db      *gorm.DB

	variable *utils.SafeMapWithKey[string, []int64]
	member   *utils.SafeMapWithKey[string, []int64]
	class    *utils.SafeMapWithKey[string, []int64]
	consts   *utils.SafeMapWithKey[string, []int64]

	indexSaver  *dbcache.Save[*ssadb.IrIndex]
	offsetSaver *dbcache.Save[*ssadb.IrOffset]

	// offsetSaved deduplicates offset rows by composite key
	// (value_id + file_hash + start_offset + end_offset). Without this,
	// compile-unit split cross-unit resolution visits the same instruction
	// from the same file editor multiple times, creating truly identical
	// offset rows. Different ranges for the same value_id (e.g. instruction
	// appearing in different files) are legitimate and are NOT deduped.
	offsetSavedMu sync.Mutex
	offsetSaved   map[string]struct{}
}

func newIndexStore(cfg *ssaconfig.Config, prog *Program, mode ProgramCacheKind, db *gorm.DB, saveSize int) *indexStore {
	saveSize = resolveAuxiliarySaveSize(cfg, saveSize)
	store := &indexStore{
		mode:        mode,
		program:     prog,
		db:          db,
		variable:    utils.NewSafeMapWithKey[string, []int64](),
		member:      utils.NewSafeMapWithKey[string, []int64](),
		class:       utils.NewSafeMapWithKey[string, []int64](),
		consts:      utils.NewSafeMapWithKey[string, []int64](),
		offsetSaved: make(map[string]struct{}),
	}
	if mode != ProgramCacheDBWrite || db == nil {
		return store
	}

	store.indexSaver = dbcache.NewSave(func(indices []*ssadb.IrIndex) error {
		saveStep := func() error {
			return utils.GormTransaction(db, func(tx *gorm.DB) error {
				batch := make([]*ssadb.IrIndex, 0, len(indices))
				for _, index := range indices {
					if index != nil {
						batch = append(batch, index)
					}
				}
				ssadb.SaveIrIndexBatch(tx, batch)
				return nil
			})
		}
		return store.diagnosticsTrackErr("ssa.Database.SaveIrIndexBatch", saveStep)
	},
		dbcache.WithSaveSize(saveSize),
		dbcache.WithSaveTimeout(saveTime),
		dbcache.WithName("IrIndex"),
	)
	store.offsetSaver = dbcache.NewSave(func(offsets []*ssadb.IrOffset) error {
		saveStep := func() error {
			return utils.GormTransaction(db, func(tx *gorm.DB) error {
				return ssadb.SaveIrOffsetBatch(tx, offsets)
			})
		}
		return store.diagnosticsTrackErr("ssa.Database.SaveIrOffsetBatch", saveStep)
	},
		dbcache.WithSaveSize(saveSize),
		dbcache.WithSaveTimeout(saveTime),
		dbcache.WithName("IrOffset"),
	)
	return store
}

func (s *indexStore) Close() error {
	if s == nil {
		return nil
	}
	var errs []error
	if s.indexSaver != nil {
		if err := s.indexSaver.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.offsetSaver != nil {
		if err := s.offsetSaver.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	// The dedup map only needs to live for one program compile: after the
	// offset saver is closed, all rows have been handed to the writer, so
	// clearing it bounds memory instead of growing with unique offsets across
	// programs (review A3). Clearing mid-compile (Flush) is unsafe because a
	// later re-visit would re-enqueue duplicates that the DB UNIQUE index
	// rejects.
	s.offsetSavedMu.Lock()
	s.offsetSaved = nil
	s.offsetSavedMu.Unlock()
	return utils.JoinErrors(errs...)
}

func (s *indexStore) Flush() error {
	if s == nil {
		return nil
	}
	var errs []error
	if s.indexSaver != nil {
		if err := s.indexSaver.Flush(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.offsetSaver != nil {
		if err := s.offsetSaver.Flush(); err != nil {
			errs = append(errs, err)
		}
	}
	return utils.JoinErrors(errs...)
}

func (s *indexStore) AddInstructionOffsets(inst Instruction) {
	if s == nil || s.offsetSaver == nil || utils.IsNil(inst) {
		return
	}
	if offset := ConvertValue2Offset(inst); offset != nil {
		s.saveOffsetDedup(offset)
	}
}

func (s *indexStore) AddConst(inst Instruction) {
	if s == nil || utils.IsNil(inst) {
		return
	}
	appendResidentIndex(s.consts, inst.GetName(), inst.GetId())
}

func (s *indexStore) AddVariable(name string, inst Instruction) {
	if s == nil || utils.IsNil(inst) {
		return
	}
	name, member := normalizeVariableName(name)
	if member != "" {
		appendResidentIndex(s.member, member, inst.GetId())
		if s.indexSaver != nil {
			s.indexSaver.Save(CreateVariableIndexByMember(member, inst))
		}
		return
	}

	appendResidentIndex(s.variable, name, inst.GetId())
	if s.indexSaver != nil {
		s.indexSaver.Save(CreateVariableIndexByName(name, inst))
	}
}

func (s *indexStore) RemoveVariable(name string, inst Instruction) {
	if s == nil || utils.IsNil(inst) {
		return
	}
	name, member := normalizeVariableName(name)
	if member != "" {
		removeResidentIndex(s.member, member, inst.GetId())
		return
	}
	removeResidentIndex(s.variable, name, inst.GetId())
}

func (s *indexStore) AddClassInstance(name string, inst Instruction) {
	if s == nil || utils.IsNil(inst) {
		return
	}
	appendResidentIndex(s.class, name, inst.GetId())
	if s.indexSaver != nil {
		s.indexSaver.Save(CreateClassIndex(name, inst))
	}
}

func (s *indexStore) FindByVariableEx(mod ssadb.MatchMode, checkValue func(string) bool, resolve func(id int64) Instruction) []Instruction {
	if s == nil || resolve == nil {
		return nil
	}
	var ins []Instruction
	appendResolved := func(ids []int64) {
		for _, id := range ids {
			if id <= 0 {
				continue
			}
			inst := resolve(id)
			if inst == nil {
				continue
			}
			ins = append(ins, inst)
		}
	}
	if mod&ssadb.ConstType != 0 {
		s.consts.ForEach(func(_ string, ids []int64) bool {
			for _, id := range ids {
				if id <= 0 {
					continue
				}
				inst := resolve(id)
				if inst == nil {
					continue
				}
				if checkValue(inst.String()) {
					ins = append(ins, inst)
				}
			}
			return true
		})
		return ins
	}
	if mod&ssadb.KeyMatch != 0 {
		s.member.ForEach(func(key string, instructions []int64) bool {
			if checkValue(key) {
				appendResolved(instructions)
			}
			return true
		})
	}
	if mod&ssadb.NameMatch != 0 {
		s.variable.ForEach(func(key string, instructions []int64) bool {
			if checkValue(key) {
				appendResolved(instructions)
			}
			return true
		})
		s.class.ForEach(func(key string, instructions []int64) bool {
			if checkValue(key) {
				appendResolved(instructions)
			}
			return true
		})
	}
	return ins
}

func (s *indexStore) diagnosticsTrack(name string, steps ...func() error) {
	_ = s.diagnosticsTrackErr(name, steps...)
}

func (s *indexStore) diagnosticsTrackErr(name string, steps ...func() error) error {
	if s == nil || s.program == nil {
		for _, step := range steps {
			if step != nil {
				if err := step(); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return s.program.DiagnosticsTrackErr(name, steps...)
}

func (s *indexStore) SaveVariableOffset(variable *Variable, rng *memedit.Range) {
	if s == nil || s.offsetSaver == nil || utils.IsNil(variable) || utils.IsNil(rng) {
		return
	}
	if offset := CreateVariableOffset(variable, rng); offset != nil {
		s.saveOffsetDedup(offset)
	}
}

// saveOffsetDedup saves an offset only if an identical row (same value_id +
// file_hash + start_offset + end_offset + variable_name, matching the DB
// UNIQUE index) has not already been enqueued. This prevents duplicate offset
// rows from compile-unit split cross-unit resolution visiting the same
// instruction from the same file editor multiple times. Different ranges or
// different variable names for the same value_id are legitimate and are NOT
// deduped.
func (s *indexStore) saveOffsetDedup(offset *ssadb.IrOffset) {
	if offset == nil {
		return
	}
	key := offsetDedupKey(offset.ValueID, offset.FileHash, offset.StartOffset, offset.EndOffset, offset.VariableName)
	s.offsetSavedMu.Lock()
	if s.offsetSaved == nil {
		s.offsetSaved = make(map[string]struct{})
	}
	if _, ok := s.offsetSaved[key]; ok {
		s.offsetSavedMu.Unlock()
		return
	}
	s.offsetSavedMu.Unlock()
	// Mark only after the enqueue attempt: if the saver cannot accept the item
	// here (failures are surfaced later by Flush/Close), the offset is not
	// poisoned and a later visit can retry it.
	s.offsetSaver.Save(offset)
	s.offsetSavedMu.Lock()
	s.offsetSaved[key] = struct{}{}
	s.offsetSavedMu.Unlock()
}

// offsetDedupKey creates a composite key for offset deduplication.
func offsetDedupKey(valueID int64, fileHash string, start, end int64, variableName string) string {
	return strconv.FormatInt(valueID, 10) + "|" + fileHash + "|" +
		strconv.FormatInt(start, 10) + "|" + strconv.FormatInt(end, 10) + "|" + variableName
}

func appendResidentIndex(index *utils.SafeMapWithKey[string, []int64], key string, id int64) {
	if index == nil || id <= 0 {
		return
	}
	data, ok := index.Get(key)
	if !ok {
		data = make([]int64, 0, 1)
	}
	data = append(data, id)
	index.Set(key, data)
}

func removeResidentIndex(index *utils.SafeMapWithKey[string, []int64], key string, id int64) {
	if index == nil || id <= 0 {
		return
	}
	data, ok := index.Get(key)
	if !ok {
		return
	}
	data = utils.RemoveSliceItem(data, id)
	index.Set(key, data)
}

func CreateVariableIndexByName(name string, inst Instruction) *ssadb.IrIndex {
	return CreateVariableIndex(inst, name, "")
}

func CreateVariableIndexByMember(member string, inst Instruction) *ssadb.IrIndex {
	return CreateVariableIndex(inst, "", member)
}

func CreateVariableIndex(inst Instruction, name, member string) *ssadb.IrIndex {
	if utils.IsNil(inst) {
		return nil
	}
	if inst.GetId() == -1 {
		return nil
	}
	prog := inst.GetProgram()
	if utils.IsNil(prog) || utils.IsNil(prog.GetApplication()) || utils.IsNil(prog.NameCache) {
		return nil
	}
	progName := prog.GetApplication().GetProgramName()

	index := ssadb.CreateIndex(progName)
	index.ProgramName = prog.GetApplication().Name
	index.ValueID = inst.GetId()
	id := prog.NameCache.GetID(name)
	index.VariableID = &id

	value, ok := inst.(Value)
	if !ok {
		return nil
	}
	variable := value.GetVariable(name)
	if variable != nil {
		index.VersionID = variable.GetVersion()
		if scope := variable.GetScope(); scope != nil {
			index.ScopeName = scope.GetScopeName()
		}
	}

	fieldID := prog.NameCache.GetID(member)
	index.FieldID = &fieldID
	return index
}

func CreateClassIndex(name string, inst Instruction) *ssadb.IrIndex {
	if inst.GetId() == -1 {
		return nil
	}
	prog := inst.GetProgram()
	progName := prog.GetApplication().GetProgramName()

	index := ssadb.CreateIndex(progName)
	index.ProgramName = prog.GetApplication().Name
	index.ValueID = inst.GetId()
	classID := prog.NameCache.GetID(name)
	index.ClassID = &classID
	return index
}
