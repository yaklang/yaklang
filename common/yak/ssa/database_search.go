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

		insts := prog.Cache.findByVariableEx(matchMode, check)
		if len(excludeFiles) == 0 {
			addRes(insts...)
			return
		}

		filteredInsts := make([]Instruction, 0, len(insts))
		excludeSet := make(map[string]struct{}, len(excludeFiles)*4)
		addExcludeKey := func(path string) {
			if path == "" {
				return
			}
			n := normalizeFilePathForExclude(path)
			excludeSet[n] = struct{}{}
			excludeSet[strings.TrimPrefix(n, "/")] = struct{}{}
		}
		for _, excludePath := range excludeFiles {
			addExcludeKey(excludePath)
			if prog != nil && prog.Name != "" {
				addExcludeKey(stripProgramNamePrefixForExclude(excludePath, prog.Name))
			}
		}
		for _, inst := range insts {
			filePath := getInstructionFilePath(inst)
			if filePath == "" {
				filteredInsts = append(filteredInsts, inst)
				continue
			}
			normalizedPath := normalizeFilePathForExclude(filePath)
			shouldExclude := false
			if _, ok := excludeSet[normalizedPath]; ok {
				shouldExclude = true
			} else if _, ok := excludeSet[strings.TrimPrefix(normalizedPath, "/")]; ok {
				shouldExclude = true
			} else if prog != nil && prog.Name != "" {
				stripped := normalizeFilePathForExclude(stripProgramNamePrefixForExclude(filePath, prog.Name))
				if _, ok := excludeSet[stripped]; ok {
					shouldExclude = true
				} else if _, ok := excludeSet[strings.TrimPrefix(stripped, "/")]; ok {
					shouldExclude = true
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

// stripProgramNamePrefixForExclude mirrors ssaapi.normalizeOverlayFilePath's
// program-prefix strip so memory-mode exclude matches overlay excludeFiles.
func stripProgramNamePrefixForExclude(filePath, programName string) string {
	if filePath == "" || programName == "" {
		return filePath
	}
	path := strings.TrimPrefix(filePath, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return filePath
	}
	first := parts[0]
	if first == programName || strings.HasPrefix(first, programName+"(") {
		if len(parts) > 1 {
			return "/" + strings.Join(parts[1:], "/")
		}
		return "/"
	}
	return filePath
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
