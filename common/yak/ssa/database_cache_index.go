package ssa

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dbcache"
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
}

func newIndexStore(cfg *ssaconfig.Config, prog *Program, mode ProgramCacheKind, db *gorm.DB, saveSize int) *indexStore {
	saveSize = resolveAuxiliarySaveSize(cfg, saveSize)
	store := &indexStore{
		mode:     mode,
		program:  prog,
		db:       db,
		variable: utils.NewSafeMapWithKey[string, []int64](),
		member:   utils.NewSafeMapWithKey[string, []int64](),
		class:    utils.NewSafeMapWithKey[string, []int64](),
		consts:   utils.NewSafeMapWithKey[string, []int64](),
	}
	if mode != ProgramCacheDBWrite || db == nil {
		return store
	}

	store.indexSaver = dbcache.NewSave(func(indices []*ssadb.IrIndex) {
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
		store.diagnosticsTrack("ssa.Database.SaveIrIndexBatch", saveStep)
	},
		dbcache.WithSaveSize(saveSize),
		dbcache.WithSaveTimeout(saveTime),
	)
	store.offsetSaver = dbcache.NewSave(func(offsets []*ssadb.IrOffset) {
		saveStep := func() error {
			return utils.GormTransaction(db, func(tx *gorm.DB) error {
				for _, offset := range offsets {
					if offset == nil {
						continue
					}
					ssadb.SaveIrOffset(tx, offset)
				}
				return nil
			})
		}
		store.diagnosticsTrack("ssa.Database.SaveIrOffsetBatch", saveStep)
	},
		dbcache.WithSaveSize(saveSize),
		dbcache.WithSaveTimeout(saveTime),
	)
	return store
}

func (s *indexStore) Close() {
	if s == nil {
		return
	}
	if s.indexSaver != nil {
		s.indexSaver.Close()
	}
	if s.offsetSaver != nil {
		s.offsetSaver.Close()
	}
}

func (s *indexStore) AddInstructionOffsets(inst Instruction) {
	if s == nil || s.offsetSaver == nil || utils.IsNil(inst) {
		return
	}
	if offset := ConvertValue2Offset(inst); offset != nil {
		s.offsetSaver.Save(offset)
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
	if s.offsetSaver == nil {
		return
	}
	value, ok := inst.(Value)
	if !ok {
		return
	}
	variable := value.GetVariable(name)
	if utils.IsNil(variable) {
		return
	}
	for _, offset := range ConvertVariable2Offset(variable, name, value.GetId()) {
		s.offsetSaver.Save(offset)
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
	if s == nil || s.program == nil {
		for _, step := range steps {
			if step != nil {
				_ = step()
			}
		}
		return
	}
	s.program.DiagnosticsTrack(name, steps...)
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
