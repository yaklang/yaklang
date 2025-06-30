package ssa

import (
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/databasex"
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
	instructions *utils.SafeMapWithKey[string, []Instruction]
}

func NewInstructionsIndexMem() *InstructionsIndexMem {
	return &InstructionsIndexMem{
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

func (c *InstructionsIndexMem) Close() {

}

type InstructionsIndexItem struct {
	Name string
	Inst Instruction
}
type InstructionsIndexDB struct {
	save *databasex.Save[InstructionsIndexItem]
}

func NewInstructionsIndexDB(
	save func([]InstructionsIndexItem),
) *InstructionsIndexDB {
	return &InstructionsIndexDB{
		save: databasex.NewSave(save),
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

func NewInstructionIndex(enable bool, saveFunc func([]InstructionsIndexItem)) InstructionsIndex {
	if enable {
		return NewInstructionsIndexDB(saveFunc)
	} else {
		return NewInstructionsIndexMem()
	}
}

func (c *ProgramCache) initIndex(databaseEnable bool) {
	c.VariableIndex = NewInstructionIndex(
		databaseEnable,
		func(items []InstructionsIndexItem) {
			utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
				for _, item := range items {
					SaveVariableIndexByName(tx, item.Name, item.Inst)
				}
				return nil
			})
		},
	)
	c.MemberIndex = NewInstructionIndex(
		databaseEnable,
		func(items []InstructionsIndexItem) {
			utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
				for _, item := range items {
					SaveVariableIndexByMember(tx, item.Name, item.Inst)
				}
				return nil
			})
		},
	)

	c.ClassIndex = NewInstructionIndex(
		databaseEnable,
		func(items []InstructionsIndexItem) {
			utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
				for _, item := range items {
					SaveClassIndex(tx, item.Name, item.Inst)
				}
				return nil
			})
		},
	)

	c.OffsetCache = NewInstructionIndex(
		databaseEnable,
		func(items []InstructionsIndexItem) {
			utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
				for _, item := range items {
					SaveValueOffset(tx, item.Inst)
					if value, ok := ToValue(item.Inst); ok {
						for _, variable := range value.GetAllVariables() {
							if variable.GetId() <= 0 {
								continue // skip variable without id
							}
							SaveVariableOffset(tx, variable, variable.GetName(), int64(value.GetId()))
						}
					}
				}
				return nil
			})
		},
	)

	c.ConstCache = NewInstructionIndex(
		databaseEnable,
		func(ii []InstructionsIndexItem) {
		},
	)
}
