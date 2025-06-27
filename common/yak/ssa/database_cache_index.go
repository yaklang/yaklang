package ssa

import (
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
	save *databasex.Saver[InstructionsIndexItem]
}

func NewInstructionsIndexDB(
	save func([]InstructionsIndexItem),
) *InstructionsIndexDB {
	return &InstructionsIndexDB{
		save: databasex.NewSaver(save),
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

}
