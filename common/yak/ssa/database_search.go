package ssa

import (
	"context"
	"regexp"
	"strings"

	"github.com/gobwas/glob"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"golang.org/x/exp/slices"
)

func MatchInstructionByOpcodes(ctx context.Context, prog *Program, opcodes ...Opcode) []Instruction {
	var insts []Instruction
	_ = diagnostics.TrackLow("ssa.MatchInstructionByOpcodes", func() error {
		insts = matchInstructionByOpcodes(ctx, prog, opcodes...)
		return nil
	})
	return insts
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

func MatchInstructionsByVariable(
	ctx context.Context,
	prog *Program,
	compareMode ssadb.CompareMode,
	matchMode ssadb.MatchMode,
	name string,
) (res []Instruction) {
	return MatchInstructionsByVariableWithExcludeFiles(ctx, prog, compareMode, matchMode, name, nil)
}

// MatchInstructionsByVariableWithExcludeFiles 搜索变量，支持排除指定文件
// excludeFiles: 要排除的文件路径列表（规范化后的路径，如 "/test.go"）
func MatchInstructionsByVariableWithExcludeFiles(
	ctx context.Context,
	prog *Program,
	compareMode ssadb.CompareMode,
	matchMode ssadb.MatchMode,
	name string,
	excludeFiles []string,
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

	loadFromMemory := func() {
		var check func(string) bool
		switch compareMode {
		case ssadb.ExactCompare:
			check = func(s string) bool { return s == name }
		case ssadb.GlobCompare:
			matcher, err := glob.Compile(name)
			if err != nil {
				return
			}
			check = func(s string) bool { return matcher.Match(s) }
		case ssadb.RegexpCompare:
			matcher, err := regexp.Compile(name)
			if err != nil {
				return
			}
			check = func(s string) bool { return matcher.MatchString(s) }
		default:
			return
		}

		insts := prog.Cache._getByVariableEx(matchMode, check)
		if len(excludeFiles) == 0 {
			addRes(insts...)
			return
		}

		filteredInsts := make([]Instruction, 0, len(insts))
		for _, inst := range insts {
			filePath := getInstructionFilePath(inst)
			if filePath == "" {
				filteredInsts = append(filteredInsts, inst)
				continue
			}
			normalizedPath := normalizeFilePathForExclude(filePath)
			shouldExclude := false
			for _, excludePath := range excludeFiles {
				if normalizeFilePathForExclude(excludePath) == normalizedPath {
					shouldExclude = true
					break
				}
			}
			if !shouldExclude {
				filteredInsts = append(filteredInsts, inst)
			}
		}
		addRes(filteredInsts...)
	}

	// all application in database, just use sql
	switch prog.DatabaseKind {
	case ProgramCacheMemory:
		loadFromMemory()
	case ProgramCacheDBWrite:
		// During compile, DBWrite is a live in-memory program with asynchronous spill.
		// Querying the DB here can immediately reload just-spilled instructions and
		// amplify save -> reload -> rewrite cost. Prefer the live in-memory indexes.
		loadFromMemory()
	case ProgramCacheDBRead:
		ch := ssadb.SearchVariableWithExcludeFiles(ssadb.GetDBInProgram(prog.Name), ctx, prog.Name, prog.NameCache, compareMode, matchMode, name, excludeFiles)
		for ir := range ch {
			var inst Instruction
			var err error
			inst, err = NewLazyInstructionFromIrCode(ir, prog)
			if err != nil {
				log.Errorf("NewLazyInstructionFromIrCode failed: %v", err)
				continue
			}
			addRes(inst)
		}
	}
	return ret
}

// normalizeFilePathForExclude 规范化文件路径用于排除匹配
func normalizeFilePathForExclude(path string) string {
	if path == "" {
		return ""
	}
	// 确保以 / 开头
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

// getInstructionFilePath 获取指令的文件路径
func getInstructionFilePath(inst Instruction) string {
	if inst == nil {
		return ""
	}
	// 尝试从指令的 Range 获取文件路径
	if r := inst.GetRange(); r != nil {
		if editor := r.GetEditor(); editor != nil {
			return editor.GetUrl()
		}
	}
	return ""
}

func (c *ProgramCache) _getByVariableEx(
	mod ssadb.MatchMode,
	checkValue func(string) bool,
) []Instruction {
	var ins []Instruction
	appendResolved := func(ids []int64) {
		for _, id := range ids {
			if id <= 0 {
				continue
			}
			inst := c.GetInstruction(id)
			if inst == nil {
				continue
			}
			ins = append(ins, inst)
		}
	}
	if mod&ssadb.ConstType != 0 {
		c.ConstCache.ForEach(func(_ string, ids []int64) {
			for _, id := range ids {
				if id <= 0 {
					continue
				}
				inst := c.GetInstruction(id)
				if inst == nil {
					continue
				}
				if checkValue(inst.String()) {
					ins = append(ins, inst)
				}
			}
		})
		return ins
	}
	if mod&ssadb.KeyMatch != 0 {
		// search all instruction
		c.MemberIndex.ForEach(func(s string, instructions []int64) {
			if checkValue(s) {
				appendResolved(instructions)
			}
		})
	}
	if mod&ssadb.NameMatch != 0 {
		// search in variable cache
		c.VariableIndex.ForEach(func(s string, instruction []int64) {
			if checkValue(s) {
				appendResolved(instruction)
			}
		})

		// search in class instance
		c.ClassIndex.ForEach(func(s string, instruction []int64) {
			if checkValue(s) {
				appendResolved(instruction)
			}
		})
	}
	return ins
}
