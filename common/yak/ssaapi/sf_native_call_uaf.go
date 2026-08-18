package ssaapi

import (
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
)

// NativeCall_UAF finds use-after-free sites via lifetime analysis.
// Double-free is included as a UAF subtype (second free of a Freed object).
// Usage:
//   - *<uaf()> as $uaf                      // all UAF / double-free sites
//   - $ptr<uaf()> as $uaf                   // findings related to $ptr
//   - <uaf(target=$ptr)> as $uaf            // same, named target (receiver may be *)
const NativeCall_UAF = "uaf"

func nativeCallUAF(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	prog, err := fetchProgram(vs)
	if err != nil || prog == nil || prog.Program == nil {
		return false, sfvm.NewEmptyValues(), utils.Errorf("uaf: no program context: %v", err)
	}

	targetSeeds, targetSpecified := resolveLifetimeTargetSeeds(frame, params)
	seeds := collectSSAValues(vs)
	if targetSpecified {
		seeds = targetSeeds
	}

	var findings []*lifetime.Finding
	if !targetSpecified && (receiverIsProgramOnly(vs) || len(seeds) == 0) {
		findings = lifetime.FindUAFUses(prog.Program)
	} else {
		findings = lifetime.FindUAFUsesRelated(prog.Program, seeds)
	}

	results := make([]sfvm.ValueOperator, 0, len(findings))
	seen := make(map[int64]struct{})
	for _, f := range findings {
		if f == nil || f.Use == nil {
			continue
		}
		if f.Kind != lifetime.KindUAF && f.Kind != lifetime.KindDoubleFree {
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

func collectSSAValues(vs sfvm.Values) []ssa.Value {
	if vs == nil {
		return nil
	}
	var seeds []ssa.Value
	_ = vs.Recursive(func(operator sfvm.ValueOperator) error {
		v, ok := operator.(*Value)
		if !ok || v == nil {
			return nil
		}
		if iv := v.getValue(); iv != nil {
			seeds = append(seeds, iv)
		}
		return nil
	})
	return seeds
}

func receiverIsProgramOnly(vs sfvm.Values) bool {
	onlyProgram := true
	_ = vs.Recursive(func(operator sfvm.ValueOperator) error {
		switch operator.(type) {
		case *Program:
		default:
			onlyProgram = false
		}
		return nil
	})
	return onlyProgram
}

// resolveLifetimeTargetSeeds reads target=$ptr (also positional 0 / var).
// specified is true when the native call named a target, even if the symbol is empty.
func resolveLifetimeTargetSeeds(frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (seeds []ssa.Value, specified bool) {
	if params == nil {
		return nil, false
	}
	raw := strings.TrimSpace(params.GetString(0, "target", "var"))
	if raw == "" && !params.Existed("target") && !params.Existed("var") && !params.Existed(0) {
		return nil, false
	}
	name := strings.TrimPrefix(raw, "$")
	if name == "" || frame == nil {
		return nil, true
	}
	vals, ok := frame.GetSymbolByName(name)
	if !ok || vals == nil {
		return nil, true
	}
	return collectSSAValues(vals), true
}
