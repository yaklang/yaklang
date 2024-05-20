package ssa

import (
	"regexp"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func (c *Cache) GetByVariableExact(isMember bool, name string) []Instruction {
	return c._getByVariableEx(isMember,
		func() chan int64 { return ssadb.ExactSearchVariable(c.DB, isMember, name) },
		func(s string) bool { return s == name },
	)
}

// GetByVariableGlob means get variable name(glob).
func (c *Cache) GetByVariableGlob(isMember bool, g sfvm.Glob) []Instruction {
	return c._getByVariableEx(isMember,
		func() chan int64 { return ssadb.GlobSearchVariable(c.DB, isMember, g.String()) },
		func(s string) bool { return g.Match(s) },
	)
}

// GetByVariableRegexp will filter Instruction via variable regexp name
func (c *Cache) GetByVariableRegexp(isMember bool, r *regexp.Regexp) []Instruction {
	return c._getByVariableEx(isMember,
		func() chan int64 { return ssadb.RegexpSearchVariable(c.DB, isMember, r.String()) },
		func(s string) bool { return r.MatchString(s) },
	)
}

func (c *Cache) _getByVariableEx(
	isMember bool,
	checkValueFromDB func() chan int64,
	checkValue func(string) bool,
) []Instruction {
	if c.HaveDatabaseBackend() {
		var insts []Instruction
		for id := range checkValueFromDB() {
			inst := c.newLazyInstruction(id)
			insts = append(insts, inst)
		}
		return insts
	}

	var ins []Instruction
	if isMember {
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
			_ = inst
		})
	} else {
		c.VariableCache.ForEach(func(s string, instructions []Instruction) {
			if checkValue(s) {
				ins = append(ins, instructions...)
			}
		})
	}
	return ins
}
