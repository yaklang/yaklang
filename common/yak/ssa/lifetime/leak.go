package lifetime

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// mergeStatesMayAlive is the leak dual of mergeStates: an object stays Alive
// if ANY predecessor path still has it Alive (may-leak). Freed only when all
// preds Freed. pointsTo is still unioned.
func mergeStatesMayAlive(states []*pathState) *pathState {
	if len(states) == 0 {
		return newPathState()
	}
	out := newPathState()
	for _, s := range states {
		if s == nil {
			continue
		}
		for oid, st := range s.objects {
			prev, ok := out.objects[oid]
			if !ok {
				out.objects[oid] = st
			} else if st == StateAlive || prev == StateAlive {
				out.objects[oid] = StateAlive
			} else {
				out.objects[oid] = StateFreed
			}
			if out.objects[oid] == StateFreed {
				if fc, ok := s.killedBy[oid]; ok {
					out.killedBy[oid] = fc
				}
			} else {
				delete(out.killedBy, oid)
			}
		}
		for vid, objs := range s.pointsTo {
			dst := out.pointsTo[vid]
			if dst == nil {
				dst = make(map[int64]struct{})
				out.pointsTo[vid] = dst
			}
			for o := range objs {
				dst[o] = struct{}{}
			}
		}
	}
	return out
}

// FindMemLeaks reports heap allocations that may still be Alive at a function
// exit without escaping via return (ownership transfer). Formal parameters are
// not reported (caller's responsibility).
func FindMemLeaks(prog *ssa.Program) []*Finding {
	if prog == nil {
		return nil
	}
	st := getState(prog)
	if st == nil {
		return nil
	}
	st.leakOnce.Do(func() {
		st.ensureIndex(prog)
		sum := st.ensureSummaries(prog)
		reg := st.reg
		var findings []*Finding
		seen := make(map[int64]struct{})
		add := func(f *Finding) {
			if f == nil || f.Use == nil || f.Use.GetId() <= 0 {
				return
			}
			if _, ok := seen[f.Use.GetId()]; ok {
				return
			}
			seen[f.Use.GetId()] = struct{}{}
			findings = append(findings, f)
		}
		prog.EachFunction(func(fn *ssa.Function) {
			defer func() { _ = recover() }()
			for _, f := range analyzeFunctionLeak(fn, reg, sum.freeParams, sum.freeDerefParams, sum.derefParams) {
				add(f)
			}
		})
		st.leakFindings = findings
	})
	return st.leakFindings
}

// FindMemLeaksRelated filters leak findings related to seed pointer/alloc values.
func FindMemLeaksRelated(prog *ssa.Program, seeds []ssa.Value) []*Finding {
	if len(seeds) == 0 {
		return nil
	}
	seedIDs := expandLifetimeSeedIDs(seeds)
	if len(seedIDs) == 0 {
		return nil
	}
	all := FindMemLeaks(prog)
	reg := getReg(prog)
	var out []*Finding
	for _, f := range all {
		if findingRelatedToSeeds(f, seedIDs, reg) {
			out = append(out, f)
		}
	}
	return out
}

func analyzeFunctionLeak(fn *ssa.Function, reg *registry, freeParams, freeDerefParams, derefParams map[int64]map[int]struct{}) []*Finding {
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}
	allocs := discoverAllocs(fn, reg)
	if len(allocs) == 0 {
		return nil
	}
	// Empty entry: do NOT seed formals as Alive for leak (avoids reporting params).
	entrySeed := newPathState()

	blocks := make([]*ssa.BasicBlock, 0, len(fn.Blocks))
	blockIndex := make(map[int64]int)
	for _, bid := range fn.Blocks {
		b, ok := fn.GetBasicBlockByID(bid)
		if !ok || b == nil {
			continue
		}
		blockIndex[b.GetId()] = len(blocks)
		blocks = append(blocks, b)
	}
	if len(blocks) == 0 {
		return nil
	}

	inState := make([]*pathState, len(blocks))
	outState := make([]*pathState, len(blocks))
	for i := range blocks {
		inState[i] = newPathState()
		outState[i] = newPathState()
	}

	entryID := fn.EnterBlock
	if entryID <= 0 && len(fn.Blocks) > 0 {
		entryID = fn.Blocks[0]
	}

	work := make([]int, 0, len(blocks))
	inWork := make([]bool, len(blocks))
	enqueue := func(i int) {
		if i < 0 || i >= len(blocks) || inWork[i] {
			return
		}
		inWork[i] = true
		work = append(work, i)
	}
	for i := range blocks {
		enqueue(i)
	}

	maxIter := len(blocks)*8 + 8
	for iter := 0; len(work) > 0 && iter < maxIter; iter++ {
		i := work[0]
		work = work[1:]
		inWork[i] = false
		b := blocks[i]

		preds := make([]*pathState, 0, len(b.Preds)+1)
		if b.GetId() == entryID {
			preds = append(preds, entrySeed.clone())
		} else if len(b.Preds) == 0 {
			preds = append(preds, newPathState())
		}
		for _, pid := range b.Preds {
			if idx, ok := blockIndex[pid]; ok {
				preds = append(preds, outState[idx])
			}
		}
		merged := mergeStatesMayAlive(preds)
		if !equalState(inState[i], merged) {
			inState[i] = merged
		}
		st := inState[i].clone()
		_ = transferBlock(b, st, allocs, reg, freeParams, freeDerefParams, derefParams)
		if !equalState(outState[i], st) {
			outState[i] = st
			for _, sid := range b.Succs {
				if idx, ok := blockIndex[sid]; ok {
					enqueue(idx)
				}
			}
		}
	}

	var findings []*Finding
	seenObj := make(map[int64]struct{})
	for i, b := range blocks {
		if !isExitBlock(b) {
			continue
		}
		st := outState[i]
		if st == nil {
			continue
		}
		escaped := collectEscapedViaReturn(b, st)
		for oid, state := range st.objects {
			if state != StateAlive {
				continue
			}
			if _, ok := allocs[oid]; !ok {
				continue
			}
			if _, ok := escaped[oid]; ok {
				continue
			}
			if _, ok := seenObj[oid]; ok {
				continue
			}
			seenObj[oid] = struct{}{}
			use := resolveProgValue(fn.GetProgram(), oid)
			if use == nil {
				// Fall back to any return in the block as the report site.
				use = firstReturnValue(b)
			}
			if use == nil {
				continue
			}
			findings = append(findings, &Finding{
				Use:      use,
				FreedObj: oid,
				Kind:     KindLeak,
			})
		}
	}
	return findings
}

func isExitBlock(b *ssa.BasicBlock) bool {
	if b == nil {
		return false
	}
	if len(b.Succs) == 0 {
		return true
	}
	for _, iid := range b.Insts {
		inst, ok := b.GetInstructionById(iid)
		if !ok || inst == nil {
			continue
		}
		if _, ok := ssa.ToReturn(resolveInstruction(inst)); ok {
			return true
		}
	}
	return false
}

func collectEscapedViaReturn(b *ssa.BasicBlock, st *pathState) map[int64]struct{} {
	escaped := make(map[int64]struct{})
	if b == nil || st == nil {
		return escaped
	}
	for _, iid := range b.Insts {
		inst, ok := b.GetInstructionById(iid)
		if !ok || inst == nil {
			continue
		}
		ret, ok := ssa.ToReturn(resolveInstruction(inst))
		if !ok || ret == nil {
			continue
		}
		for _, rid := range ret.Results {
			rv, ok := ret.GetValueById(rid)
			if !ok || rv == nil {
				continue
			}
			for oid := range st.objsOf(rv.GetId()) {
				escaped[oid] = struct{}{}
			}
			escaped[rv.GetId()] = struct{}{}
			if pid := paramObjectID(rv); pid > 0 {
				escaped[pid] = struct{}{}
			}
		}
	}
	return escaped
}

func firstReturnValue(b *ssa.BasicBlock) ssa.Value {
	if b == nil {
		return nil
	}
	for _, iid := range b.Insts {
		inst, ok := b.GetInstructionById(iid)
		if !ok || inst == nil {
			continue
		}
		if ret, ok := ssa.ToReturn(resolveInstruction(inst)); ok && ret != nil {
			return ret
		}
	}
	return nil
}
