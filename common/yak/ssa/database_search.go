package ssa

import (
	"context"
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"regexp"
)

func MatchInstructionByExact(ctx context.Context, prog *Program, mod int, e string) []Instruction {
	return matchInstructionsByVariable(ctx, prog, ssadb.ExactCompare, mod, e)
}

// GetByVariableGlob means get variable name(glob).
func MatchInstructionByGlob(ctx context.Context, prog *Program, mod int, g string) []Instruction {
	return matchInstructionsByVariable(ctx, prog, ssadb.GlobCompare, mod, g)
}

// GetByVariableRegexp will filter Instruction via variable regexp name
func MatchInstructionByRegexp(ctx context.Context, prog *Program, mod int, r string) []Instruction {
	return matchInstructionsByVariable(ctx, prog, ssadb.RegexpCompare, mod, r)
}

func MatchInstructionByOpcode(ctx context.Context, prog *Program, opcode string) []Instruction {
	return matchInstructionByOpcode(ctx, prog, opcode)
}

func matchInstructionByOpcode(ctx context.Context, prog *Program, opcodeName string) []Instruction {
	var insts []Instruction

	if prog.EnableDatabase {
		ch := ssadb.SearchIrCodeByOpcode(ssadb.GetDBInProgram(prog.Name), opcodeName, ctx)
		for ir := range ch {
			inst, err := NewLazyInstructionFromIrCode(ir, prog)
			if err != nil {
				log.Errorf("NewLazyInstructionFromIrCode failed: %v", err)
				continue
			}
			insts = append(insts, inst)
		}
		return insts
	}

	checkOpcode := func(op Opcode) bool {
		return SSAOpcode2Name[op] == opcodeName
	}
	for _, cache := range prog.Cache.InstructionCache.GetAll() {
		inst := cache.inst
		value, b := ToValue(cache.inst)
		if b && checkOpcode(value.GetOpcode()) {
			insts = append(insts, inst)
		}
	}
	return insts
}

func matchInstructionsByVariable(
	ctx context.Context,
	prog *Program,
	compareMode, matchMode int,
	name string,
) []Instruction {
	// all application in database, just use sql
	if prog.EnableDatabase {
		var insts []Instruction
		ch := ssadb.SearchVariable(ssadb.GetDBInProgram(prog.Name), ctx, compareMode, matchMode, name)
		for ir := range ch {
			inst, err := NewLazyInstructionFromIrCode(ir, prog)
			if err != nil {
				log.Errorf("NewLazyInstructionFromIrCode failed: %v", err)
				continue
			}
			// inst := prog.Cache.newLazyInstructionWithoutCache(int64(id.ID))
			insts = append(insts, inst)
		}
		return insts
	}

	res := make([]Instruction, 0)
	tmp := make(map[int64]struct{})
	addRes := func(insts ...Instruction) {
		for _, inst := range insts {
			if _, ok := tmp[inst.GetId()]; !ok {
				res = append(res, inst)
				tmp[inst.GetId()] = struct{}{}
			}
		}
	}

	// from cache
	check := func(s string) bool {
		switch compareMode {
		case ssadb.ExactCompare:
			return s == name
		case ssadb.GlobCompare:
			return glob.MustCompile(name).Match(s)
		case ssadb.RegexpCompare:
			return regexp.MustCompile(name).MatchString(s)
		}
		return false
	}
	addRes(prog.Cache._getByVariableEx(matchMode, check)...)
	return res
}

func (c *Cache) _getByVariableEx(
	mod int,
	checkValue func(string) bool,
) []Instruction {
	var ins []Instruction
	if mod&ssadb.ConstType != 0 {
		for _, instruction := range c.constCache {
			value, b := ToValue(instruction)
			if b && checkValue(value.String()) {
				ins = append(ins, instruction)
			}
		}
		return ins
	}
	if mod&ssadb.KeyMatch != 0 {
		// search all instruction
		for member, insts := range c.MemberCache {
			if checkValue(member) {
				ins = append(ins, insts...)
			}
		}
	}
	if mod&ssadb.NameMatch != 0 {
		// search in variable cache
		for name, insts := range c.VariableCache {
			if checkValue(name) {
				ins = append(ins, insts...)
			}
		}

		// search in class instance
		for name, insts := range c.Class2InstIndex {
			if checkValue(name) {
				ins = append(ins, insts...)
			}
		}
	}
	return ins
}
