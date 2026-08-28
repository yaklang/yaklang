package lifetime

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// FindNullChecks returns SSA condition values used in null / non-null tests
// (if (p), if (!p), p == NULL, p != NULL, …).
func FindNullChecks(prog *ssa.Program) []ssa.Value {
	if prog == nil {
		return nil
	}
	seen := make(map[int64]struct{})
	var out []ssa.Value
	add := func(v ssa.Value) {
		if v == nil || v.GetId() <= 0 {
			return
		}
		if _, ok := seen[v.GetId()]; ok {
			return
		}
		seen[v.GetId()] = struct{}{}
		out = append(out, v)
	}
	prog.EachFunction(func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		defer func() { _ = recover() }()
		for _, bid := range fn.Blocks {
			b, ok := fn.GetBasicBlockByID(bid)
			if !ok || b == nil {
				continue
			}
			for _, iid := range b.Insts {
				inst, ok := b.GetInstructionById(iid)
				if !ok || inst == nil {
					continue
				}
				ifInst, ok := ssa.ToIfInstruction(resolveInstruction(inst))
				if !ok || ifInst == nil {
					continue
				}
				cond, ok := ifInst.GetValueById(ifInst.Cond)
				if !ok || cond == nil {
					continue
				}
				target, _ := resolveNullCheckTarget(cond)
				if target == nil || target.GetId() <= 0 {
					continue
				}
				add(cond)
			}
		}
	})
	return out
}

// FindNullChecksRelated returns null-check conditions whose tested pointer
// relates to the given seeds.
func FindNullChecksRelated(prog *ssa.Program, seeds []ssa.Value) []ssa.Value {
	if len(seeds) == 0 {
		return nil
	}
	seedIDs := expandLifetimeSeedIDs(seeds)
	if len(seedIDs) == 0 {
		return nil
	}
	var out []ssa.Value
	for _, cond := range FindNullChecks(prog) {
		target, _ := resolveNullCheckTarget(cond)
		if target == nil {
			continue
		}
		if _, ok := seedIDs[target.GetId()]; ok {
			out = append(out, cond)
			continue
		}
		if pid := paramObjectID(target); pid > 0 {
			if _, ok := seedIDs[pid]; ok {
				out = append(out, cond)
			}
		}
	}
	return out
}
