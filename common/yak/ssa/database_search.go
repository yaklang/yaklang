package ssa

import (
	"context"
	"regexp"

	"github.com/gobwas/glob"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"golang.org/x/exp/slices"
)

func MatchInstructionByOpcodes(ctx context.Context, prog *Program, opcodes ...Opcode) []Instruction {
	return MatchInstructionByOpcodesWithFileFilter(ctx, prog, nil, nil, opcodes...)
}

// MatchInstructionByOpcodesWithFileFilter matches opcodes then applies include/exclude file filters.
// Empty file path is kept (extern/lib). includeFiles takes precedence when non-empty.
func MatchInstructionByOpcodesWithFileFilter(
	ctx context.Context,
	prog *Program,
	includeFiles, excludeFiles []string,
	opcodes ...Opcode,
) []Instruction {
	var insts []Instruction
	_ = diagnostics.TrackLow("ssa.MatchInstructionByOpcodes", func() error {
		insts = matchInstructionByOpcodes(ctx, prog, opcodes...)
		if len(includeFiles) > 0 || len(excludeFiles) > 0 {
			insts = filterInstructionsByFiles(prog, insts, includeFiles, excludeFiles)
		}
		return nil
	})
	return insts
}

func matchInstructionByOpcodes(ctx context.Context, prog *Program, opcodes ...Opcode) []Instruction {
	var insts []Instruction
	switch prog.DatabaseKind {
	case ProgramCacheMemory:
		for _, inst := range prog.Cache.residentInstructions() {
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

func filterInstructionsByFiles(prog *Program, insts []Instruction, includeFiles, excludeFiles []string) []Instruction {
	if len(insts) == 0 || (len(includeFiles) == 0 && len(excludeFiles) == 0) {
		return insts
	}
	progName := ""
	if prog != nil {
		progName = prog.Name
	}
	includeSet := ssadb.BuildFilePathSet(includeFiles, progName)
	excludeSet := ssadb.BuildFilePathSet(excludeFiles, progName)
	out := make([]Instruction, 0, len(insts))
	for _, inst := range insts {
		filePath := getInstructionFilePath(inst)
		if len(includeSet) > 0 && !ssadb.PathPassesFileFilter(filePath, includeSet, ssadb.FileFilterInclude, progName) {
			continue
		}
		if !ssadb.PathPassesFileFilter(filePath, excludeSet, ssadb.FileFilterExclude, progName) {
			continue
		}
		out = append(out, inst)
	}
	return out
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
	return matchInstructionsByVariableWithFileFilter(ctx, prog, compareMode, matchMode, name, excludeFiles, nil)
}

// MatchInstructionsByVariableWithIncludeFiles 搜索变量，仅保留 includeFiles 中的结果
func MatchInstructionsByVariableWithIncludeFiles(
	ctx context.Context,
	prog *Program,
	compareMode ssadb.CompareMode,
	matchMode ssadb.MatchMode,
	name string,
	includeFiles []string,
) (res []Instruction) {
	return matchInstructionsByVariableWithFileFilter(ctx, prog, compareMode, matchMode, name, nil, includeFiles)
}

func matchInstructionsByVariableWithFileFilter(
	ctx context.Context,
	prog *Program,
	compareMode ssadb.CompareMode,
	matchMode ssadb.MatchMode,
	name string,
	excludeFiles []string,
	includeFiles []string,
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

		insts := prog.Cache.findByVariableEx(matchMode, check)
		addRes(filterInstructionsByFiles(prog, insts, includeFiles, excludeFiles)...)
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
		var ch <-chan *ssadb.IrCode
		if len(includeFiles) > 0 {
			ch = ssadb.SearchVariableWithIncludeFiles(ssadb.GetDBInProgram(prog.Name), ctx, prog.Name, prog.NameCache, compareMode, matchMode, name, includeFiles)
		} else {
			ch = ssadb.SearchVariableWithExcludeFiles(ssadb.GetDBInProgram(prog.Name), ctx, prog.Name, prog.NameCache, compareMode, matchMode, name, excludeFiles)
		}
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

// getInstructionFilePath 获取指令的文件路径（不含 program-name 前缀，便于 exclude 对齐）
func getInstructionFilePath(inst Instruction) string {
	if inst == nil {
		return ""
	}
	if r := inst.GetRange(); r != nil {
		if editor := r.GetEditor(); editor != nil {
			// Prefer FilePath (no program name) so excludeFiles from overlay match.
			if fp := editor.GetFilePath(); fp != "" {
				return fp
			}
			return editor.GetUrl()
		}
	}
	return ""
}
