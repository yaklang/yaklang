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
//   - free?(* #-> <uaf()>) as $free         // keep free() calls whose pointer has UAF
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

func forEachReceiverValue(vs sfvm.Values, fn func(op sfvm.ValueOperator, inner ssa.Value)) {
	if vs == nil || fn == nil {
		return
	}
	_ = vs.Recursive(func(operator sfvm.ValueOperator) error {
		v, ok := operator.(*Value)
		if !ok || v == nil {
			return nil
		}
		inner := v.getValue()
		if inner == nil {
			return nil
		}
		fn(operator, inner)
		return nil
	})
}

func ssaValuesToSFVM(prog *Program, vals []ssa.Value) (map[int64]*Value, []sfvm.ValueOperator) {
	id2val := make(map[int64]*Value, len(vals))
	results := make([]sfvm.ValueOperator, 0, len(vals))
	if prog == nil {
		return id2val, results
	}
	for _, iv := range vals {
		if iv == nil || iv.GetId() <= 0 {
			continue
		}
		id := iv.GetId()
		if _, ok := id2val[id]; ok {
			continue
		}
		val, err := prog.NewValue(iv)
		if err != nil || val == nil {
			continue
		}
		id2val[id] = val
		results = append(results, val)
	}
	return id2val, results
}

func finishLifetimeSFVM(results []sfvm.ValueOperator) (bool, sfvm.Values, error) {
	if len(results) == 0 {
		return false, sfvm.NewEmptyValues(), nil
	}
	return true, sfvm.NewValues(results), nil
}

func appendLifetimePredecessor(dst *Value, src sfvm.ValueOperator, frame *sfvm.SFFrame, label string) {
	if dst == nil || frame == nil || utils.IsNil(src) {
		return
	}
	if sv, ok := src.(*Value); ok && sv != nil && dst.getValue() != nil && sv.getValue() != nil {
		if dst.getValue().GetId() == sv.getValue().GetId() {
			return
		}
	}
	_ = dst.AppendPredecessor(src, frame.WithPredecessorContext(label))
}

func newSSAPredecessor(prog *Program, iv ssa.Value) *Value {
	if prog == nil || iv == nil || iv.GetId() <= 0 {
		return nil
	}
	val, err := prog.NewValue(iv)
	if err != nil || val == nil {
		return nil
	}
	return val
}

func attachFindingGraph(prog *Program, frame *sfvm.SFFrame, opName string, f *lifetime.Finding, val *Value) {
	if val == nil || f == nil {
		return
	}
	if f.FreeCall != nil {
		if fc := newSSAPredecessor(prog, f.FreeCall); fc != nil {
			appendLifetimePredecessor(val, fc, frame, opName+":free")
		}
	}
	if f.FreedObj > 0 && (f.Use == nil || f.Use.GetId() != f.FreedObj) && prog != nil {
		if obj, err := prog.GetValueById(f.FreedObj); err == nil && obj != nil {
			appendLifetimePredecessor(val, obj, frame, opName+":alloc")
		}
	}
}

// propagateRelatedSSAAnchors copies each receiver's anchor bits onto result
// values related to that receiver (so NewValue-based natives work in ?{} /
// func?(...) filters) and hangs the receiver as a predecessor for the
// IRify audit-process graph.
func propagateRelatedSSAAnchors(vs sfvm.Values, related func(inner ssa.Value) []ssa.Value, id2val map[int64]*Value, frame *sfvm.SFFrame, predLabel string) {
	if related == nil || len(id2val) == 0 {
		return
	}
	forEachReceiverValue(vs, func(op sfvm.ValueOperator, inner ssa.Value) {
		for _, iv := range related(inner) {
			if iv == nil {
				continue
			}
			if val, ok := id2val[iv.GetId()]; ok && val != nil {
				sfvm.MergeAnchor(op, val)
				appendLifetimePredecessor(val, op, frame, predLabel)
			}
		}
	})
}

func attachFallbackCallPredecessor(prog *Program, frame *sfvm.SFFrame, label string, val *Value) {
	if val == nil || len(val.Predecessors) > 0 {
		return
	}
	inner := val.getValue()
	if inner == nil {
		return
	}
	try := func(iv ssa.Value) bool {
		if iv == nil || iv.GetId() <= 0 || iv.GetId() == inner.GetId() {
			return false
		}
		pred := newSSAPredecessor(prog, iv)
		if pred == nil {
			return false
		}
		appendLifetimePredecessor(val, pred, frame, label)
		return len(val.Predecessors) > 0
	}
	if inner.HasValues() {
		for _, op := range inner.GetValues() {
			if call, ok := ssa.ToCall(op); ok && call != nil && try(call) {
				return
			}
		}
		for _, op := range inner.GetValues() {
			if try(op) {
				return
			}
		}
	}
	for _, u := range inner.GetUsers() {
		if u == nil {
			continue
		}
		if call, ok := ssa.ToCall(u); ok && call != nil && try(call) {
			return
		}
	}
}

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
	id2val := make(map[int64]*Value, len(findings))
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
		if _, ok := id2val[id]; ok {
			continue
		}
		val, err := prog.NewValue(f.Use)
		if err != nil || val == nil {
			continue
		}
		attachFindingGraph(prog, frame, opName, f, val)
		id2val[id] = val
		results = append(results, val)
	}
	if len(results) == 0 {
		return false, sfvm.NewEmptyValues(), nil
	}
	// Per-receiver bits so <uaf()> can be used in ?{} / func?(...) filters.
	// Also copy the receiver as a predecessor so the audit-process graph is non-empty.
	// Skip full-scan / target=$x: results are not derived from the receiver list.
	if !targetSpecified && !fullScan {
		propagateRelatedSSAAnchors(vs, func(inner ssa.Value) []ssa.Value {
			var out []ssa.Value
			for _, f := range find(prog.Program, []ssa.Value{inner}, false) {
				if f != nil && f.Use != nil {
					out = append(out, f.Use)
				}
			}
			return out
		}, id2val, frame, opName)
	}
	return finishLifetimeSFVM(results)
}
