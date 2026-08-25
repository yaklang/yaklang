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
//   - <uaf(kind="double-free")>             // optional kind filter
const NativeCall_UAF = "uaf"

func nativeCallUAF(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return runLifetimeNativeCall(vs, frame, params, "uaf",
		func(prog *ssa.Program, seeds []ssa.Value, full bool) []*lifetime.Finding {
			if full {
				return lifetime.FindUAFUses(prog)
			}
			return lifetime.FindUAFUsesRelated(prog, seeds)
		},
		func(kind string) bool {
			return kind == lifetime.KindUAF || kind == lifetime.KindDoubleFree
		},
	)
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

type lifetimeFindFn func(prog *ssa.Program, seeds []ssa.Value, fullScan bool) []*lifetime.Finding

func runLifetimeNativeCall(
	vs sfvm.Values,
	frame *sfvm.SFFrame,
	params *sfvm.NativeCallActualParams,
	opName string,
	find lifetimeFindFn,
	kindOK func(string) bool,
) (bool, sfvm.Values, error) {
	prog, err := fetchProgram(vs)
	if err != nil || prog == nil || prog.Program == nil {
		return false, sfvm.NewEmptyValues(), utils.Errorf("%s: no program context: %v", opName, err)
	}

	targetSeeds, targetSpecified := resolveLifetimeTargetSeeds(frame, params)
	seeds := collectSSAValues(vs)
	if targetSpecified {
		seeds = targetSeeds
	}

	fullScan := !targetSpecified && (receiverIsProgramOnly(vs) || len(seeds) == 0)
	findings := find(prog.Program, seeds, fullScan)

	kindFilter := ""
	if params != nil {
		kindFilter = strings.ToLower(strings.TrimSpace(params.GetString("kind")))
	}

	results := make([]sfvm.ValueOperator, 0, len(findings))
	seen := make(map[int64]struct{})
	for _, f := range findings {
		if f == nil || f.Use == nil {
			continue
		}
		if !kindOK(f.Kind) {
			continue
		}
		if kindFilter != "" && f.Kind != kindFilter {
			// Allow "uaf" filter to still include double-free when caller asks kind=uaf? Plan:
			// kind="double-free" optional; default unchanged. Strict equality is fine.
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
		if f.FreeCall != nil && frame != nil {
			if fc, err := prog.NewValue(f.FreeCall); err == nil && fc != nil {
				val.AppendPredecessor(fc, frame.WithPredecessorContext(opName+":free"))
			}
		}
		results = append(results, val)
	}
	if len(results) == 0 {
		return false, sfvm.NewEmptyValues(), nil
	}
	return true, sfvm.NewValues(results), nil
}
