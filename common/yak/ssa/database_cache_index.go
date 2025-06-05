package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

type InstructionsIndex interface {
	Delete(string, Instruction) error
	Add(string, Instruction) error
	ForEach(func(string, []Instruction))
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

func (c *InstructionsIndexMem) Delete(key string, inst Instruction) error {
	data, ok := c.instructions.Get(key)
	if !ok {
		return nil
	}
	data = utils.RemoveSliceItem(data, inst)
	c.instructions.Set(key, data)
	return nil
}

func (c *InstructionsIndexMem) Add(key string, inst Instruction) error {
	data, ok := c.instructions.Get(key)
	if !ok {
		data = make([]Instruction, 0)
	}
	data = append(data, inst)
	c.instructions.Set(key, data)
	return nil
}

func (c *InstructionsIndexMem) ForEach(f func(string, []Instruction)) {
	c.instructions.ForEach(func(key string, value []Instruction) bool {
		f(key, value)
		return true
	})
}

type InstructionsIndexDB struct {
	save func(string, Instruction) error
}

func NewInstructionsIndexDB(
	save func(string, Instruction) error,
) *InstructionsIndexDB {
	return &InstructionsIndexDB{
		save: save,
	}
}

func (c *InstructionsIndexDB) Delete(key string, inst Instruction) error {
	// Implement database deletion logic here
	return nil
}

func (c *InstructionsIndexDB) Add(key string, inst Instruction) error {
	return c.save(key, inst)
}

func (c *InstructionsIndexDB) ForEach(f func(string, []Instruction)) {
	// Implement database iteration logic here
	return
}
