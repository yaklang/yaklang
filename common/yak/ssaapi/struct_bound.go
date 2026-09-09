package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// structBound is the intra-compile-unit filter used by mode=struct scans.
// Identity is the unit's library Program (functions/classes live there),
// with a file-path fallback for Application-hosted IR.
type structBound struct {
	key         string
	lib         *ssa.Program
	programName string
	fileList    []string
	files       map[string]struct{}
}

func newStructBound(unit *ssa.CompileUnit, ssaProg *ssa.Program) *structBound {
	programName := ""
	var lib *ssa.Program
	if ssaProg != nil {
		app := ssaProg.GetApplication()
		if app == nil {
			app = ssaProg
		}
		programName = app.GetProgramName()
		if unit != nil {
			lib = app.ProgramForCompileUnit(unit.Key)
		}
	}
	if unit == nil {
		return &structBound{lib: lib, programName: programName, files: map[string]struct{}{}}
	}
	return &structBound{
		key:         unit.Key,
		lib:         lib,
		programName: programName,
		fileList:    append([]string(nil), unit.Files...),
		files:       ssadb.BuildFilePathSet(unit.Files, programName),
	}
}

func (b *structBound) compileUnit() *ssa.CompileUnit {
	if b == nil {
		return nil
	}
	return &ssa.CompileUnit{Key: b.key, Files: append([]string(nil), b.fileList...)}
}

func (b *structBound) Allow(inst ssa.Instruction) bool {
	if b == nil || inst == nil {
		return true
	}
	if inst.IsExtern() || inst.GetOpcode() == ssa.SSAOpcodeExternLib {
		return true
	}
	p := inst.GetProgram()
	if b.lib != nil && p != nil {
		if p == b.lib {
			if b.lib.ProgramKind == ssa.Library {
				return true
			}
			// Application-hosted unit: the Program is shared, so isolate by file.
			return ssadb.PathPassesFileFilter(ssa.InstructionFilePath(inst), b.files, ssadb.FileFilterInclude, b.programName)
		}
		if p.ProgramKind == ssa.Library {
			return false
		}
	}
	return ssadb.PathPassesFileFilter(ssa.InstructionFilePath(inst), b.files, ssadb.FileFilterInclude, b.programName)
}

func structBoundFromConfig(cfg *sfvm.Config) *structBound {
	if cfg == nil {
		return nil
	}
	for _, opt := range cfg.RuntimeOptions {
		if b, ok := opt.(*structBound); ok {
			return b
		}
	}
	return nil
}

func WithStructBound(bound *structBound) OperationOption {
	return func(operationConfig *OperationConfig) {
		operationConfig.structBound = bound
	}
}

func filterValuesByStructBound(bound *structBound, vals Values) Values {
	if bound == nil || len(vals) == 0 {
		return vals
	}
	out := make(Values, 0, len(vals))
	for _, v := range vals {
		if v == nil {
			continue
		}
		if inst := v.getValue(); inst != nil && !bound.Allow(inst) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func filterSFVMValuesByStructBound(bound *structBound, vals sfvm.Values) sfvm.Values {
	if bound == nil || len(vals) == 0 {
		return vals
	}
	var out []sfvm.ValueOperator
	_ = vals.Recursive(func(operator sfvm.ValueOperator) error {
		switch ret := operator.(type) {
		case *Value:
			if ret != nil {
				if inst := ret.getValue(); inst != nil && !bound.Allow(inst) {
					return nil
				}
			}
			out = append(out, ret)
		default:
			out = append(out, operator)
		}
		return nil
	})
	return sfvm.NewValues(out)
}
