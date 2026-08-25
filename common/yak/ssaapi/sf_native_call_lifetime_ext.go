package ssaapi

import (
	"strings"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
)

// NativeCall_MemLeak finds heap allocations that may leak at function exit.
// Usage:
//   - *<memLeak()> as $leak
//   - $ptr<memLeak()> / <memLeak(target=$ptr)>
const NativeCall_MemLeak = "memLeak"

// NativeCall_NullCheck returns null/non-null branch conditions (if (p), p==NULL, …).
// Usage:
//   - *<nullCheck()> as $chk
//   - $ptr<nullCheck()> / <nullCheck(target=$ptr)>
const NativeCall_NullCheck = "nullCheck"

// NativeCall_PointsTo returns may-points-to objects of the receiver pointer(s).
// Usage:
//   - $p<pointsTo()> as $obj
const NativeCall_PointsTo = "pointsTo"

// NativeCall_Aliases keeps receiver values that may-alias target.
// Usage:
//   - $a<aliases(target=$b)> as $hit
const NativeCall_Aliases = "aliases"

func nativeCallMemLeak(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return runLifetimeNativeCall(vs, frame, params, "memLeak",
		func(prog *ssa.Program, seeds []ssa.Value, full bool) []*lifetime.Finding {
			if full {
				return lifetime.FindMemLeaks(prog)
			}
			return lifetime.FindMemLeaksRelated(prog, seeds)
		},
		func(kind string) bool {
			return kind == lifetime.KindLeak
		},
	)
}

func nativeCallNullCheck(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return runLifetimeQueryNativeCall(vs, frame, params, "nullCheck",
		lifetime.FindNullChecks, lifetime.FindNullChecksRelated)
}

func nativeCallPointsTo(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	prog, err := fetchProgram(vs)
	if err != nil || prog == nil || prog.Program == nil {
		return false, sfvm.NewEmptyValues(), utils.Errorf("pointsTo: no program context: %v", err)
	}
	targetSeeds, targetSpecified := resolveLifetimeTargetSeeds(frame, params)
	seeds := collectSSAValues(vs)
	if targetSpecified {
		seeds = targetSeeds
	}
	if len(seeds) == 0 {
		return false, sfvm.NewEmptyValues(), nil
	}
	vals := lifetime.PointsToRelated(prog.Program, seeds)
	return valuesToSFVM(prog, vals)
}

func nativeCallAliases(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	prog, err := fetchProgram(vs)
	if err != nil || prog == nil || prog.Program == nil {
		return false, sfvm.NewEmptyValues(), utils.Errorf("aliases: no program context: %v", err)
	}
	seeds := collectSSAValues(vs)
	against, specified := resolveLifetimeTargetSeeds(frame, params)
	if !specified {
		// Also accept against= / alias=
		if params != nil {
			raw := strings.TrimSpace(params.GetString("against", "alias"))
			if raw != "" {
				name := strings.TrimPrefix(raw, "$")
				if name != "" && frame != nil {
					if vals, ok := frame.GetSymbolByName(name); ok {
						against = collectSSAValues(vals)
						specified = true
					}
				}
			}
		}
	}
	if !specified || len(against) == 0 || len(seeds) == 0 {
		return false, sfvm.NewEmptyValues(), nil
	}
	vals := lifetime.AliasesOf(prog.Program, seeds, against)
	return valuesToSFVM(prog, vals)
}

func valuesToSFVM(prog *Program, vals []ssa.Value) (bool, sfvm.Values, error) {
	results := make([]sfvm.ValueOperator, 0, len(vals))
	seen := make(map[int64]struct{})
	for _, iv := range vals {
		if iv == nil || iv.GetId() <= 0 {
			continue
		}
		id := iv.GetId()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		val, err := prog.NewValue(iv)
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
