package ssa

import (
	"context"
	"regexp"

	"github.com/gobwas/glob"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"golang.org/x/exp/slices"
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

func MatchInstructionByOpcodes(ctx context.Context, prog *Program, opcodes ...Opcode) []Instruction {
	return matchInstructionByOpcodes(ctx, prog, opcodes...)
}

func matchInstructionByOpcodes(ctx context.Context, prog *Program, opcodes ...Opcode) []Instruction {
	var insts []Instruction
	switch prog.DatabaseKind {
	case ProgramCacheMemory:
		for _, inst := range prog.Cache.InstructionCache.GetAll() {
			if slices.Contains(opcodes, inst.GetOpcode()) {
				insts = append(insts, inst)
			}
		}
	case ProgramCacheDBRead, ProgramCacheDBWrite:
		ch := ssadb.SearchIrCodeByOpcodes(ssadb.GetDBInProgram(prog.Name), ctx,
			prog.Name,
			lo.Map(opcodes, func(opcode Opcode, index int) int {
				return int(opcode)
			})...,
		)
		for ir := range ch {
			inst, err := NewLazyInstructionFromIrCode(ir, prog)
			if err != nil {
				log.Errorf("NewLazyInstructionFromIrCode failed: %v", err)
				continue
			}
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
) (res []Instruction) {
	var ret []Instruction
	tmp := make(map[int64]struct{})
	addRes := func(insts ...Instruction) {
		for _, inst := range insts {
			if _, ok := tmp[inst.GetId()]; !ok {
				ret = append(ret, inst)
				tmp[inst.GetId()] = struct{}{}
			}
		}
	}
	// all application in database, just use sql
	switch prog.DatabaseKind {
	case ProgramCacheMemory:
		// from cache
		var check func(string) bool
		// check := func(s string) bool {
		switch compareMode {
		case ssadb.ExactCompare:
			check = func(s string) bool { return s == name }
		case ssadb.GlobCompare:
			matcher, err := glob.Compile(name)
			if err != nil {
				return
			}
			check = func(s string) bool {
				return matcher.Match(s)
			}
		case ssadb.RegexpCompare:
			matcher, err := regexp.Compile(name)
			if err != nil {
				return
			}
			check = func(s string) bool { return matcher.MatchString(s) }
		default:
			return
		}
		addRes(prog.Cache._getByVariableEx(matchMode, check)...)
	case ProgramCacheDBRead, ProgramCacheDBWrite:
		ch := ssadb.SearchVariable(ssadb.GetDBInProgram(prog.Name), ctx, prog.Name, compareMode, matchMode, name)
		for ir := range ch {
			inst, err := NewLazyInstructionFromIrCode(ir, prog)
			if err != nil {
				log.Errorf("NewLazyInstructionFromIrCode failed: %v", err)
				continue
			}
			// inst := prog.Cache.newLazyInstructionWithoutCache(int64(id.ID))
			addRes(inst)
		}
	}
	return ret
}

func (c *ProgramCache) _getByVariableEx(
	mod int,
	checkValue func(string) bool,
) []Instruction {
	var ins []Instruction
	if mod&ssadb.ConstType != 0 {
		c.ConstCache.ForEach(func(s string, instruction []Instruction) {
			for _, i := range instruction {
				if checkValue(i.String()) {
					ins = append(ins, i)
				}
			}
		})
		return ins
	}
	if mod&ssadb.KeyMatch != 0 {
		// search all instruction
		c.MemberIndex.ForEach(func(s string, instructions []Instruction) {
			if checkValue(s) {
				ins = append(ins, instructions...)
			}
		})
	}
	if mod&ssadb.NameMatch != 0 {
		// search in variable cache
		c.VariableIndex.ForEach(func(s string, instruction []Instruction) {
			if checkValue(s) {
				ins = append(ins, instruction...)
			}
		})

		// search in class instance
		c.ClassIndex.ForEach(func(s string, instruction []Instruction) {
			if checkValue(s) {
				ins = append(ins, instruction...)
			}
		})
	}
	return ins
}
