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

func (c *InstructionsIndexMem) Close() {

}

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
	save func([]InstructionsIndexItem),
) *InstructionsIndexDB {
	return &InstructionsIndexDB{
		save: databasex.NewSave(
			save,
			databasex.WithName(name),
			databasex.WithSaveSize(IndexSaveSize),
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

func NewInstructionIndex(enable bool, name string, saveFunc func([]InstructionsIndexItem)) InstructionsIndex {
	if enable {
		return NewInstructionsIndexDB(name, saveFunc)
	} else {
		return NewInstructionsIndexMem(name)
	}
}

func (c *ProgramCache) initIndex(databaseEnable bool) {

	c.VariableIndex = NewInstructionIndex(
		databaseEnable, "VariableIndex",
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
		databaseEnable, "MemberIndex",
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
		databaseEnable, "ClassIndex",
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
		databaseEnable, "OffsetCache",
		func(items []InstructionsIndexItem) {
			irOffset := make([]*ssadb.IrOffset, 0, len(items)*2)
			add := func(i ...*ssadb.IrOffset) {
				for _, item := range i {
					if !utils.IsNil(item) {
						irOffset = append(irOffset, item)
					}
				}
			}
			defer func() {
				log.Errorf("DATABASE: Save IR Offsets: %d", len(irOffset))
				utils.GormTransaction(c.DB, func(tx *gorm.DB) error {
					for _, item := range irOffset {
						ssadb.SaveIrOffset(tx, item)
					}
					return nil
				})
			}()
			for _, item := range items {
				add(SaveValueOffset(item.Inst))
				if value, ok := ToValue(item.Inst); ok {
					for _, variable := range value.GetAllVariables() {
						if variable.GetId() <= 0 {
							continue // skip variable without id
						}
						add(SaveVariableOffset(variable, variable.GetName(), int64(value.GetId()))...)
					}
				}
			}
		},
	)

	c.ConstCache = NewInstructionIndex(
		databaseEnable, "ConstCache",
		func(ii []InstructionsIndexItem) {
		},
	)

}
