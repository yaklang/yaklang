package lifetime

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// PointsTo returns values that seed may point to (may-points-to, union across
// CFG outs in the seed's function). Empty when unknown.
func PointsTo(prog *ssa.Program, seed ssa.Value) []ssa.Value {
	if prog == nil || seed == nil || seed.GetId() <= 0 {
		return nil
	}
	byFunc := ensurePointsToIndex(prog)
	fn := seed.GetFunc()
	if fn == nil {
		return nil
	}
	m := byFunc[fn.GetId()]
	if m == nil {
		return nil
	}
	objs := m[seed.GetId()]
	if len(objs) == 0 {
		// Also consult param object id / self.
		if pid := paramObjectID(seed); pid > 0 {
			objs = m[pid]
		}
	}
	return resolveObjectIDs(prog, objs)
}

// PointsToRelated returns pointed-to objects for any of the seeds.
func PointsToRelated(prog *ssa.Program, seeds []ssa.Value) []ssa.Value {
	seen := make(map[int64]struct{})
	var out []ssa.Value
	for _, s := range seeds {
		for _, v := range PointsTo(prog, s) {
			if v == nil || v.GetId() <= 0 {
				continue
			}
			if _, ok := seen[v.GetId()]; ok {
				continue
			}
			seen[v.GetId()] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}

// MayAlias reports whether a and b may refer to a common abstract object
// (conservative may-alias). Same value id is always true.
func MayAlias(prog *ssa.Program, a, b ssa.Value) bool {
	if a == nil || b == nil {
		return false
	}
	if a.GetId() > 0 && a.GetId() == b.GetId() {
		return true
	}
	oa := objectSetOf(prog, a)
	ob := objectSetOf(prog, b)
	if len(oa) == 0 || len(ob) == 0 {
		return false
	}
	for id := range oa {
		if _, ok := ob[id]; ok {
			return true
		}
	}
	return false
}

// AliasesOf returns seeds that may-alias againstTarget (for native $a<aliases(target=$b)>).
func AliasesOf(prog *ssa.Program, seeds []ssa.Value, against []ssa.Value) []ssa.Value {
	if len(seeds) == 0 || len(against) == 0 {
		return nil
	}
	var out []ssa.Value
	seen := make(map[int64]struct{})
	for _, s := range seeds {
		if s == nil || s.GetId() <= 0 {
			continue
		}
		hit := false
		for _, t := range against {
			if MayAlias(prog, s, t) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		if _, ok := seen[s.GetId()]; ok {
			continue
		}
		seen[s.GetId()] = struct{}{}
		out = append(out, s)
	}
	return out
}

func objectSetOf(prog *ssa.Program, v ssa.Value) map[int64]struct{} {
	out := make(map[int64]struct{})
	if v == nil || v.GetId() <= 0 {
		return out
	}
	out[v.GetId()] = struct{}{}
	if pid := paramObjectID(v); pid > 0 {
		out[pid] = struct{}{}
	}
	byFunc := ensurePointsToIndex(prog)
	fn := v.GetFunc()
	if fn == nil {
		return out
	}
	m := byFunc[fn.GetId()]
	if m == nil {
		return out
	}
	for oid := range m[v.GetId()] {
		out[oid] = struct{}{}
	}
	if pid := paramObjectID(v); pid > 0 {
		for oid := range m[pid] {
			out[oid] = struct{}{}
		}
	}
	return out
}

func resolveObjectIDs(prog *ssa.Program, objs map[int64]struct{}) []ssa.Value {
	var out []ssa.Value
	seen := make(map[int64]struct{})
	for oid := range objs {
		if oid <= 0 {
			continue
		}
		if _, ok := seen[oid]; ok {
			continue
		}
		seen[oid] = struct{}{}
		if v := resolveProgValue(prog, oid); v != nil {
			out = append(out, v)
		}
	}
	return out
}

func ensurePointsToIndex(prog *ssa.Program) map[int64]map[int64]map[int64]struct{} {
	st := getState(prog)
	if st == nil {
		return nil
	}
	st.pointsOnce.Do(func() {
		st.ensureIndex(prog)
		sum := st.ensureSummaries(prog)
		st.pointsByFunc = make(map[int64]map[int64]map[int64]struct{})
		prog.EachFunction(func(fn *ssa.Function) {
			if fn == nil {
				return
			}
			defer func() { _ = recover() }()
			outs := computeFunctionPointsTo(fn, st.reg, sum.freeParams, sum.freeDerefParams, sum.derefParams)
			if len(outs) > 0 {
				st.pointsByFunc[fn.GetId()] = outs
			}
		})
	})
	return st.pointsByFunc
}

// computeFunctionPointsTo runs UAF-style dataflow (Freed-wins merge) and returns
// the union of pointsTo maps across all block OUTs: valueID -> objectIDs.
func computeFunctionPointsTo(fn *ssa.Function, reg *registry, freeParams, freeDerefParams, derefParams map[int64]map[int]struct{}) map[int64]map[int64]struct{} {
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}
	allocs := discoverAllocs(fn, reg)
	entrySeed := entryStateFor(fn)

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
		merged := mergeStates(preds)
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

	union := make(map[int64]map[int64]struct{})
	for _, st := range outState {
		if st == nil {
			continue
		}
		for vid, objs := range st.pointsTo {
			dst := union[vid]
			if dst == nil {
				dst = make(map[int64]struct{})
				union[vid] = dst
			}
			for o := range objs {
				dst[o] = struct{}{}
			}
		}
	}
	return union
}
