package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
)

// NativeCall_NPD finds null-pointer dereference sites (independent of UAF).
// Usage:
//   - *<npd()> as $npd                 // all NPD sites in the program
//   - $ptr<npd()> as $npd              // NPD related to selected pointers
const NativeCall_NPD = "npd"

func nativeCallNPD(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	prog, err := fetchProgram(vs)
	if err != nil || prog == nil || prog.Program == nil {
		return false, sfvm.NewEmptyValues(), utils.Errorf("npd: no program context: %v", err)
	}

	var seeds []ssa.Value
	onlyProgram := true
	_ = vs.Recursive(func(operator sfvm.ValueOperator) error {
		switch v := operator.(type) {
		case *Program:
		case *Value:
			onlyProgram = false
			if iv := v.getValue(); iv != nil {
				seeds = append(seeds, iv)
			}
		default:
			onlyProgram = false
		}
		return nil
	})

	var findings []*lifetime.Finding
	if onlyProgram || len(seeds) == 0 {
		findings = lifetime.FindNPDUses(prog.Program)
	} else {
		findings = lifetime.FindNPDUsesRelated(prog.Program, seeds)
	}

	results := make([]sfvm.ValueOperator, 0, len(findings))
	seen := make(map[int64]struct{})
	for _, f := range findings {
		if f == nil || f.Use == nil {
			continue
		}
		if f.Kind != lifetime.KindNPD {
			continue
		}
		id := f.Use.GetId()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		val, err := prog.NewValue(f.Use)
		if err != nil || val == nil {
			continue
		}
		results = append(results, val)
	}
	if len(results) == 0 {
		return false, sfvm.NewEmptyValues(), nil
	}
	return true, sfvm.NewValues(results), nil
}
