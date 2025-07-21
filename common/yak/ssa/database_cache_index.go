package ssa

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/databasex"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type InstructionsIndex interface {
	Delete(string, Instruction)
	Add(string, Instruction)
	ForEach(func(string, []Instruction))
	Close()
}

var _ InstructionsIndex = (*InstructionsIndexMem)(nil)
var _ InstructionsIndex = (*InstructionsIndexDB)(nil)

type InstructionsIndexMem struct {
	name         string
	instructions *utils.SafeMapWithKey[string, []Instruction]
}

func NewInstructionsIndexMem(name string) *InstructionsIndexMem {
	return &InstructionsIndexMem{
		name:         name,
		instructions: utils.NewSafeMapWithKey[string, []Instruction](),
	}
}

func (c *InstructionsIndexMem) Delete(key string, inst Instruction) {
	data, ok := c.instructions.Get(key)
	if !ok {
		return
	}
	data = utils.RemoveSliceItem(data, inst)
	c.instructions.Set(key, data)
	return
}

func (c *InstructionsIndexMem) Add(key string, inst Instruction) {
	data, ok := c.instructions.Get(key)
	if !ok {
		data = make([]Instruction, 0)
	}
	data = append(data, inst)
	c.instructions.Set(key, data)
}

func (c *InstructionsIndexMem) ForEach(f func(string, []Instruction)) {
	c.instructions.ForEach(func(key string, value []Instruction) bool {
		f(key, value)
		return true
	})
}

func (c *InstructionsIndexMem) Close() {}

type InstructionsIndexItem struct {
	Name string
	Inst Instruction
}
type InstructionsIndexDB struct {
	save *databasex.Save[InstructionsIndexItem]
}

const (
	IndexSaveSize = 2000
)

func NewInstructionsIndexDB(
	name string,
	saveSize int,
	save func([]InstructionsIndexItem),
) *InstructionsIndexDB {
	if saveSize < IndexSaveSize {
		saveSize = IndexSaveSize // Ensure minimum save size
	}
	return &InstructionsIndexDB{
		save: databasex.NewSave(
			save,
			databasex.WithName(name),
			databasex.WithSaveSize(saveSize),
			databasex.WithSaveTimeout(saveTime),
		),
	}
}

func (c *InstructionsIndexDB) Delete(key string, inst Instruction) {
	// Implement database deletion logic here
	return
}

func (c *InstructionsIndexDB) Add(key string, inst Instruction) {
	// return c.save(key, inst)
	c.save.Save(InstructionsIndexItem{
		Name: key,
		Inst: inst,
	})
}

func (c *InstructionsIndexDB) ForEach(f func(string, []Instruction)) {
	// Implement database iteration logic here
	return
}

func (c *InstructionsIndexDB) Close() {
	c.save.Close()
}

func NewInstructionIndex(kind ProgramCacheKind, name string, saveSize int, saveFunc func([]InstructionsIndexItem)) InstructionsIndex {
	if kind != ProgramCacheMemory {
		return NewInstructionsIndexDB(name, saveSize, saveFunc)
	} else {
		return NewInstructionsIndexMem(name)
	}
}

func (c *ProgramCache) initIndex(databaseKind ProgramCacheKind, saveSize int) {
	offsetSaver := databasex.NewSave(func(t []*ssadb.IrOffset) {
		utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
			for _, item := range t {
				ssadb.SaveIrOffset(tx, item)
			}
			return nil
		})
	}, databasex.WithSaveSize(saveSize), databasex.WithName("OffsetCache"))

	saveIndex := func(db *gorm.DB, items []*ssadb.IrIndex) {
		utils.GormTransaction(db, func(tx *gorm.DB) error {
			for _, item := range items {
				ssadb.SaveIrIndex(tx, item)
			}
			return nil
		})
	}

	c.VariableIndex = NewInstructionIndex(
		databaseKind, "VariableIndex", saveSize,
		func(items []InstructionsIndexItem) {
			rets := make([]*ssadb.IrIndex, 0, len(items))
			for _, item := range items {
				ret := SaveVariableIndexByName(item.Name, item.Inst)
				rets = append(rets, ret)

				// save to offset
				if value, ok := item.Inst.(Value); ok {
					variable := value.GetVariable(item.Name)
					for _, offset := range ConvertVariable2Offset(variable, item.Name, int64(value.GetId())) {
						offsetSaver.Save(offset)
					}
				}
			}
			saveIndex(c.DB, rets)
		},
	)
	c.MemberIndex = NewInstructionIndex(
		databaseKind, "MemberIndex", saveSize,
		func(items []InstructionsIndexItem) {
			ret := make([]*ssadb.IrIndex, 0, len(items))
			for _, item := range items {
				item := SaveVariableIndexByMember(item.Name, item.Inst)
				ret = append(ret, item)
			}
			saveIndex(c.DB, ret)

		},
	)

	c.ClassIndex = NewInstructionIndex(
		databaseKind, "ClassIndex", saveSize,
		func(items []InstructionsIndexItem) {
			ret := make([]*ssadb.IrIndex, 0, len(items))
			for _, item := range items {
				item := SaveClassIndex(item.Name, item.Inst)
				ret = append(ret, item)
			}
			saveIndex(c.DB, ret)
		},
	)

	c.OffsetCache = offsetSaver

	c.ConstCache = NewInstructionIndex(
		databaseKind, "ConstCache", saveSize,
		func(ii []InstructionsIndexItem) {
		},
	)

}
