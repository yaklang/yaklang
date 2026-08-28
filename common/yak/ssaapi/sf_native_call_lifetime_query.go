package ssaapi

import (
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/lifetime"
)

// NativeCall_DoubleFree finds double-free sites (UAF subtype).
// Usage:
//   - *<doubleFree()> as $df
//   - $ptr<doubleFree()> / <doubleFree(target=$ptr)>
const NativeCall_DoubleFree = "doubleFree"

// NativeCall_HeapAlloc returns heap allocation sites (malloc/calloc/… wrappers).
// Usage:
//   - *<heapAlloc()> as $alloc
//   - $ptr<heapAlloc()> / <heapAlloc(target=$ptr)>
const NativeCall_HeapAlloc = "heapAlloc"

// NativeCall_FreeCall returns free / RegisterKill call sites.
// Usage:
//   - *<freeCall()> as $free
//   - $ptr<freeCall()> / <freeCall(target=$ptr)>
const NativeCall_FreeCall = "freeCall"

// NativeCall_DerefSite returns explicit *p load sites (RegisterDeref).
// Usage:
//   - *<derefSite()> as $d
//   - $ptr<derefSite()> / <derefSite(target=$ptr)>
const NativeCall_DerefSite = "derefSite"

func nativeCallDoubleFree(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return runLifetimeNativeCall(vs, frame, params, "doubleFree",
		func(prog *ssa.Program, seeds []ssa.Value, full bool) []*lifetime.Finding {
			if full {
				return lifetime.FindUAFUses(prog)
			}
			return lifetime.FindUAFUsesRelated(prog, seeds)
		},
		func(kind string) bool {
			return kind == lifetime.KindDoubleFree
		},
	)
}

func nativeCallHeapAlloc(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return runLifetimeQueryNativeCall(vs, frame, params, "heapAlloc",
		lifetime.ListHeapAllocs, lifetime.ListHeapAllocsRelated)
}

func nativeCallFreeCall(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return runLifetimeQueryNativeCall(vs, frame, params, "freeCall",
		lifetime.ListFreeCalls, lifetime.ListFreeCallsRelated)
}

func nativeCallDerefSite(vs sfvm.Values, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.Values, error) {
	return runLifetimeQueryNativeCall(vs, frame, params, "derefSite",
		lifetime.ListDerefSites, lifetime.ListDerefSitesRelated)
}

type lifetimeListFn func(prog *ssa.Program) []ssa.Value
type lifetimeListRelatedFn func(prog *ssa.Program, seeds []ssa.Value) []ssa.Value

func runLifetimeQueryNativeCall(
	vs sfvm.Values,
	frame *sfvm.SFFrame,
	params *sfvm.NativeCallActualParams,
	opName string,
	listAll lifetimeListFn,
	listRelated lifetimeListRelatedFn,
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

	var vals []ssa.Value
	fullScan := !targetSpecified && (receiverIsProgramOnly(vs) || len(seeds) == 0)
	if fullScan {
		vals = listAll(prog.Program)
	} else {
		vals = listRelated(prog.Program, seeds)
	}

	id2val, results := ssaValuesToSFVM(prog, vals)
	if !targetSpecified && !fullScan {
		propagateRelatedSSAAnchors(vs, func(inner ssa.Value) []ssa.Value {
			return listRelated(prog.Program, []ssa.Value{inner})
		}, id2val)
	}
	return finishLifetimeSFVM(results)
}
