package ssa

import (
	"regexp"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func (c *Cache) GetByVariableExact(mod int, name string) []Instruction {
	// return c._getByVariableEx(isMember,
	// 	func() chan int64 { return ssadb.ExactSearchVariable(c.DB, isMember, name) },
	// )
	if c.HaveDatabaseBackend() {
		var insts []Instruction
		ch := ssadb.ExactSearchVariable(c.DB, mod, name)
		for id := range ch {
			inst := c.newLazyInstruction(id)
			insts = append(insts, inst)
		}
		return insts
	}
	return c._getByVariableEx(mod,
		func(s string) bool { return s == name },
	)
}

// GetByVariableGlob means get variable name(glob).
func (c *Cache) GetByVariableGlob(mod int, g sfvm.Glob) []Instruction {
	if c.HaveDatabaseBackend() {
		var insts []Instruction
		ch := ssadb.GlobSearchVariable(c.DB, mod, g.String())
		for id := range ch {
			inst := c.newLazyInstruction(id)
			insts = append(insts, inst)
		}
		return insts
	}
	return c._getByVariableEx(mod,
		func(s string) bool { return g.Match(s) },
	)
}

// GetByVariableRegexp will filter Instruction via variable regexp name
func (c *Cache) GetByVariableRegexp(mod int, r *regexp.Regexp) []Instruction {
	if c.HaveDatabaseBackend() {
		var insts []Instruction
		ch := ssadb.RegexpSearchVariable(c.DB, mod, r.String())
		for id := range ch {
			inst := c.newLazyInstruction(id)
			insts = append(insts, inst)
		}
		return insts
	}
	return c._getByVariableEx(mod,
		func(s string) bool { return r.MatchString(s) },
	)
}

func (c *Cache) _getByVariableEx(
	mod int,
	// checkValueFromDB func() chan int64,
	checkValue func(string) bool,
) []Instruction {
	var ins []Instruction
	if mod&ssadb.KeyMatch != 0 {
		c.InstructionCache.ForEach(func(i int64, iic instructionIrCode) {
			inst := iic.inst
			value, ok := ToValue(inst)
			if !ok {
				return
			}
			if !value.IsMember() {
				return
			}
			str := value.GetKey().String()
			if checkValue(str) {
				ins = append(ins, inst)
			}
		})
	}
	if mod&ssadb.NameMatch != 0 {
		c.VariableCache.ForEach(func(s string, instructions []Instruction) {
			if checkValue(s) {
				ins = append(ins, instructions...)
			}
		})
	}
	return ins
}
