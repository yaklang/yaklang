package ssa

import (
	"github.com/yaklang/gorm"

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

	variable     *utils.SafeMapWithKey[string, []int64]
	member       *utils.SafeMapWithKey[string, []int64]
	class        *utils.SafeMapWithKey[string, []int64]
	consts       *utils.SafeMapWithKey[string, []int64]
	residentKeys *utils.SafeMapWithKey[residentIndexKey, struct{}]
	residentMu   sync.Mutex
	indexSaved   *utils.SafeMapWithKey[indexRelationshipKey, struct{}]

	indexSaver  *dbcache.Save[*ssadb.IrIndex]
	offsetSaver *dbcache.Save[*ssadb.IrOffset]

	// offsetSaved deduplicates offset rows by composite key
	// (value_id + file_hash + start_offset + end_offset + variable_name). Without this,
	// compile-unit split cross-unit resolution visits the same instruction
	// from the same file editor multiple times, creating truly identical
	// offset rows. Different ranges for the same value_id (e.g. instruction
	// appearing in different files) are legitimate and are NOT deduped.
	offsetSaved *utils.SafeMapWithKey[offsetLocationKey, struct{}]
}

func newIndexStore(cfg *ssaconfig.Config, prog *Program, mode ProgramCacheKind, db *gorm.DB, saveSize int) *indexStore {
	saveSize = resolveAuxiliarySaveSize(cfg, saveSize)
	store := &indexStore{
		mode:         mode,
		program:      prog,
		db:           db,
		variable:     utils.NewSafeMapWithKey[string, []int64](),
		member:       utils.NewSafeMapWithKey[string, []int64](),
		class:        utils.NewSafeMapWithKey[string, []int64](),
		consts:       utils.NewSafeMapWithKey[string, []int64](),
		residentKeys: utils.NewSafeMapWithKey[residentIndexKey, struct{}](),
		indexSaved:   utils.NewSafeMapWithKey[indexRelationshipKey, struct{}](),
		offsetSaved:  utils.NewSafeMapWithKey[offsetLocationKey, struct{}](),
	}
	if mode != ProgramCacheDBWrite || db == nil {
		return store
	}

	store.indexSaver = dbcache.NewSave(func(indices []*ssadb.IrIndex) error {
		saveStep := func() error {
			return utils.GormTransaction(db, func(tx *gorm.DB) error {
				return ssadb.SaveIrIndexBatch(tx, indices)
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
	s.appendIndex(s.consts, inst.GetName(), inst.GetId())
}

func (s *indexStore) AddVariable(name string, inst Instruction) {
	if s == nil || utils.IsNil(inst) {
		return
	}
	name, member := normalizeVariableName(name)
	if member != "" {
		s.appendIndex(s.member, member, inst.GetId())
		if s.indexSaver != nil {
			s.saveIndexDedup(CreateVariableIndexByMember(member, inst))
		}
		return
	}

	s.appendIndex(s.variable, name, inst.GetId())
	if s.indexSaver != nil {
		s.saveIndexDedup(CreateVariableIndexByName(name, inst))
	}
}

func (s *indexStore) RemoveVariable(name string, inst Instruction) {
	if s == nil || utils.IsNil(inst) {
		return
	}
	name, member := normalizeVariableName(name)
	s.residentMu.Lock()
	defer s.residentMu.Unlock()
	if member != "" {
		removeResidentIndex(s.member, member, inst.GetId())
		s.residentKeys.Delete(residentIndexKey{s.member, member, inst.GetId()})
		return
	}
	removeResidentIndex(s.variable, name, inst.GetId())
	s.residentKeys.Delete(residentIndexKey{s.variable, name, inst.GetId()})
}

func (s *indexStore) AddClassInstance(name string, inst Instruction) {
	if s == nil || utils.IsNil(inst) {
		return
	}
	s.appendIndex(s.class, name, inst.GetId())
	if s.indexSaver != nil {
		s.saveIndexDedup(CreateClassIndex(name, inst))
	}
}

func (s *indexStore) saveIndexDedup(index *ssadb.IrIndex) {
	if s != nil && s.indexSaver != nil && index != nil {
		fresh := false
		s.indexSaved.Update(indexRelationship(index), func(_ struct{}, exists bool) (struct{}, bool) {
			fresh = !exists
			return struct{}{}, true
		})
		if fresh {
			s.indexSaver.Save(index)
		}
	}
}

// A key owns scalar IDs, never pointers into the GORM row that the saver edits.
// The presence bits distinguish SQL NULL from an explicit zero in every ID.
type indexRelationshipKey struct {
	program, scope                                string
	value, variable, class, field, owner, version int64
	present                                       uint8
}

func indexRelationship(row *ssadb.IrIndex) indexRelationshipKey {
	k := indexRelationshipKey{program: row.ProgramName, scope: row.ScopeName, value: row.ValueID, version: row.VersionID}
	for i, id := range []*int64{row.VariableID, row.ClassID, row.FieldID, row.OwnerValueID} {
		if id != nil {
			k.present |= 1 << i
			switch i {
			case 0:
				k.variable = *id
			case 1:
				k.class = *id
			case 2:
				k.field = *id
			case 3:
				k.owner = *id
			}
		}
	}
	return k
}

type residentIndexKey struct {
	index *utils.SafeMapWithKey[string, []int64]
	name  string
	id    int64
}

func (s *indexStore) appendIndex(index *utils.SafeMapWithKey[string, []int64], name string, id int64) {
	if id <= 0 {
		return
	}
	s.residentMu.Lock()
	defer s.residentMu.Unlock()
	// Appending and publishing membership share one lock, including removal.
	s.residentKeys.Update(residentIndexKey{index, name, id}, func(_ struct{}, exists bool) (struct{}, bool) {
		if !exists {
			appendResidentIndex(index, name, id)
		}
		return struct{}{}, true
	})
}

// checkValue is a pure exact/glob/regexp predicate. Key filtering can run under
// an index read lock, but resolve (which may populate indexes) always runs after
// the matching snapshot has released that lock.
func (s *indexStore) FindByVariableEx(mod ssadb.MatchMode, checkValue func(string) bool, resolve func(id int64) Instruction) []Instruction {
	if s == nil || resolve == nil {
		return nil
	}
	var ins []Instruction
	// A value may be indexed under several matching variable/member/class
	// names. Resolve it once, before potentially reloading spilled IR from DB.
	seen := make(map[int64]struct{})
	resolveOnce := func(id int64) Instruction {
		if id <= 0 {
			return nil
		}
		if _, exists := seen[id]; exists {
			return nil
		}
		seen[id] = struct{}{}
		return resolve(id)
	}
	appendResolved := func(ids []int64) {
		for _, id := range ids {
			inst := resolveOnce(id)
			if inst == nil {
				continue
			}
			ins = append(ins, inst)
		}
	}
	if mod&ssadb.ConstType != 0 {
		for _, ids := range s.consts.Values() {
			for _, id := range ids {
				inst := resolveOnce(id)
				if inst == nil {
					continue
				}
				if checkValue(inst.String()) {
					ins = append(ins, inst)
				}
			}
		}
		return ins
	}
	if mod&ssadb.KeyMatch != 0 {
		for _, instructions := range s.member.ValuesMatching(checkValue) {
			appendResolved(instructions)
		}
	}
	if mod&ssadb.NameMatch != 0 {
		for _, instructions := range s.variable.ValuesMatching(checkValue) {
			appendResolved(instructions)
		}
		for _, instructions := range s.class.ValuesMatching(checkValue) {
			appendResolved(instructions)
		}
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

// saveOffsetDedup saves an offset only if an identical row
// (same value_id + file_hash + start_offset + end_offset + variable_name) has not already
// been saved. This prevents truly duplicate offset rows from compile-unit
// split cross-unit resolution visiting the same instruction from the same
// file editor multiple times. Different ranges for the same value_id are
// legitimate and are NOT deduped.
func (s *indexStore) saveOffsetDedup(offset *ssadb.IrOffset) {
	if offset == nil {
		return
	}
	if s.program != nil && s.program.NameCache != nil {
		offset.FileHash = s.program.NameCache.Intern(offset.FileHash)
		offset.VariableName = s.program.NameCache.Intern(offset.VariableName)
	}
	key := offsetDedupKey(offset.ValueID, offset.FileHash, offset.StartOffset, offset.EndOffset, offset.VariableName)
	fresh := false
	s.offsetSaved.Update(key, func(_ struct{}, exists bool) (struct{}, bool) {
		fresh = !exists
		return struct{}{}, true
	})
	if fresh {
		s.offsetSaver.Save(offset)
	}
}

type offsetLocationKey struct {
	value, start, end int64
	file, variable    string
}

// Match the existing DB unique key, including the variable name. IDs are
// scalars so the common duplicate lookup allocates no formatted strings.
func offsetDedupKey(valueID int64, fileHash string, start, end int64, variable ...string) offsetLocationKey {
	k := offsetLocationKey{value: valueID, file: fileHash, start: start, end: end}
	if len(variable) != 0 {
		k.variable = variable[0]
	}
	return k
}

func appendResidentIndex(index *utils.SafeMapWithKey[string, []int64], key string, id int64) {
	if index == nil || id <= 0 {
		return
	}
	index.Update(key, func(data []int64, _ bool) ([]int64, bool) {
		return append(data, id), true
	})
}

func removeResidentIndex(index *utils.SafeMapWithKey[string, []int64], key string, id int64) {
	if index == nil || id <= 0 || !index.Have(key) {
		return
	}
	index.Update(key, func(data []int64, _ bool) ([]int64, bool) {
		// Readers can retain the previous slice after releasing the map lock.
		// Removal must not shift elements in that shared backing array.
		var result []int64
		for _, item := range data {
			if item != id {
				result = append(result, item)
			}
		}
		return result, true
	})
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
