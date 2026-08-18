package lifetime

import (
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

type pathState struct {
	// object id -> state
	objects map[int64]ObjectState
	// value id -> set of object ids it may point to
	pointsTo map[int64]map[int64]struct{}
	// last free call that killed each object on this path
	killedBy map[int64]ssa.Value
}

func newPathState() *pathState {
	return &pathState{
		objects:  make(map[int64]ObjectState),
		pointsTo: make(map[int64]map[int64]struct{}),
		killedBy: make(map[int64]ssa.Value),
	}
}

func (s *pathState) clone() *pathState {
	n := newPathState()
	for k, v := range s.objects {
		n.objects[k] = v
	}
	for vid, objs := range s.pointsTo {
		cp := make(map[int64]struct{}, len(objs))
		for o := range objs {
			cp[o] = struct{}{}
		}
		n.pointsTo[vid] = cp
	}
	for k, v := range s.killedBy {
		n.killedBy[k] = v
	}
	return n
}

func mergeStates(states []*pathState) *pathState {
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
			if !ok || st == StateFreed || prev == StateFreed {
				if st == StateFreed || prev == StateFreed {
					out.objects[oid] = StateFreed
				} else {
					out.objects[oid] = StateAlive
				}
			}
			if st == StateFreed {
				if fc, ok := s.killedBy[oid]; ok {
					out.killedBy[oid] = fc
				}
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

func (s *pathState) setPointsTo(vid int64, objs map[int64]struct{}) {
	if vid <= 0 {
		return
	}
	if len(objs) == 0 {
		delete(s.pointsTo, vid)
		return
	}
	cp := make(map[int64]struct{}, len(objs))
	for o := range objs {
		cp[o] = struct{}{}
	}
	s.pointsTo[vid] = cp
}

func (s *pathState) addPointsTo(vid int64, oid int64) {
	if vid <= 0 || oid <= 0 {
		return
	}
	m := s.pointsTo[vid]
	if m == nil {
		m = make(map[int64]struct{})
		s.pointsTo[vid] = m
	}
	m[oid] = struct{}{}
}

func (s *pathState) objsOf(vid int64) map[int64]struct{} {
	return s.pointsTo[vid]
}

func callMethodName(c *ssa.Call) string {
	if c == nil {
		return ""
	}
	m, ok := c.GetValueById(c.Method)
	if !ok || m == nil {
		return ""
	}
	name := m.GetName()
	if name == "" {
		name = m.GetVerboseName()
	}
	// Extern often named like "free" or "Function-free"
	name = strings.TrimPrefix(name, "Function-")
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	return name
}

func isFreeCall(c *ssa.Call, reg *registry) bool {
	if c == nil {
		return false
	}
	if reg != nil && len(reg.killArgs(c.GetId())) > 0 {
		return true
	}
	n := strings.ToLower(callMethodName(c))
	return n == "free"
}

func equalState(a, b *pathState) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.objects) != len(b.objects) || len(a.pointsTo) != len(b.pointsTo) {
		return false
	}
	for k, v := range a.objects {
		if b.objects[k] != v {
			return false
		}
	}
	for vid, objs := range a.pointsTo {
		bo := b.pointsTo[vid]
		if len(objs) != len(bo) {
			return false
		}
		for o := range objs {
			if _, ok := bo[o]; !ok {
				return false
			}
		}
	}
	return true
}

// FindUAFUses analyzes prog and returns UAF sites, including double-free
// (treated as a UAF subtype: free of an already-Freed object).
func FindUAFUses(prog *ssa.Program) []*Finding {
	if prog == nil {
		return nil
	}
	reg := getReg(prog)
	// Callee summaries: function id -> formal param indexes that are freed.
	freeParams := buildFreeParamSummary(prog)
	freeDerefParams := buildFreeDerefParamSummary(prog)
	derefParams := buildParamDerefSummary(prog)
	var findings []*Finding
	seen := make(map[int64]struct{}) // use value id

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
		defer func() {
			if r := recover(); r != nil {
				// LazyInstruction.Self during DB-loaded programs may panic on
				// closed save channels; skip that function rather than abort all.
			}
		}()
		for _, f := range analyzeFunction(fn, reg, freeParams, freeDerefParams, derefParams) {
			add(f)
		}
	})
	return findings
}

// buildFreeParamSummary records which formal parameters each function frees
// (directly via free(param), or indirectly by calling another function that
// frees the corresponding actual). Fixpoint covers nested wrappers:
// freep2 → freep → free.
func buildFreeParamSummary(prog *ssa.Program) map[int64]map[int]struct{} {
	out := make(map[int64]map[int]struct{})
	if prog == nil {
		return out
	}
	reg := getReg(prog)

	ensure := func(fid int64) map[int]struct{} {
		m := out[fid]
		if m == nil {
			m = make(map[int]struct{})
			out[fid] = m
		}
		return m
	}

	// Pass 1: direct free(param) / free(alias-of-param) / RegisterKill on formals.
	prog.EachFunction(func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		defer func() { _ = recover() }()
		paramIndexByID := collectParamAliases(fn)
		freed := ensure(fn.GetId())
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
				inst = resolveInstruction(inst)
				call, ok := ssa.ToCall(inst)
				if !ok || call == nil || !isFreeCall(call, reg) {
					continue
				}
				argIDs := append([]int64(nil), call.Args...)
				if reg != nil {
					if ks := reg.killArgs(call.GetId()); len(ks) > 0 {
						argIDs = ks
					}
				}
				for _, aid := range argIDs {
					if idx, ok := paramIndexByID[aid]; ok {
						freed[idx] = struct{}{}
						continue
					}
					if av, ok := call.GetValueById(aid); ok && av != nil {
						if p, ok := ssa.ToParameter(av); ok && p != nil && !p.IsFreeValue {
							freed[p.FormalParameterIndex] = struct{}{}
						}
					}
				}
			}
		}
		if len(freed) == 0 {
			delete(out, fn.GetId())
		}
	})

	// Pass 2: fixpoint — calling a known free-wrapper on a formal also frees it.
	for iter := 0; iter < 16; iter++ {
		changed := false
		prog.EachFunction(func(fn *ssa.Function) {
			if fn == nil {
				return
			}
			defer func() { _ = recover() }()
			paramIndexByID := collectParamAliases(fn)
			freed := ensure(fn.GetId())
			before := len(freed)
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
					inst = resolveInstruction(inst)
					call, ok := ssa.ToCall(inst)
					if !ok || call == nil || isFreeCall(call, reg) {
						continue
					}
					for _, cid := range resolveCalleeFuncIDs(call) {
						idxs, ok := out[cid]
						if !ok || len(idxs) == 0 {
							continue
						}
						for idx := range idxs {
							if idx < 0 || idx >= len(call.Args) {
								continue
							}
							aid := call.Args[idx]
							if pidx, ok := paramIndexByID[aid]; ok {
								freed[pidx] = struct{}{}
								continue
							}
							if av, ok := call.GetValueById(aid); ok && av != nil {
								if p, ok := ssa.ToParameter(av); ok && p != nil && !p.IsFreeValue {
									freed[p.FormalParameterIndex] = struct{}{}
								}
							}
						}
					}
				}
			}
			if len(freed) == 0 {
				delete(out, fn.GetId())
			} else if len(freed) > before {
				changed = true
			}
		})
		if !changed {
			break
		}
	}
	return out
}

// buildFreeDerefParamSummary records formals that are freed through a star
// (free(*p) / free(origin-of-p)), so callers of f(&ptr) kill ptr's pointee.
func buildFreeDerefParamSummary(prog *ssa.Program) map[int64]map[int]struct{} {
	out := make(map[int64]map[int]struct{})
	if prog == nil {
		return out
	}
	reg := getReg(prog)
	ensure := func(fid int64) map[int]struct{} {
		m := out[fid]
		if m == nil {
			m = make(map[int]struct{})
			out[fid] = m
		}
		return m
	}

	prog.EachFunction(func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		defer func() { _ = recover() }()
		aliases := collectParamAliases(fn)
		freed := ensure(fn.GetId())
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
				inst = resolveInstruction(inst)
				call, ok := ssa.ToCall(inst)
				if !ok || call == nil || !isFreeCall(call, reg) {
					continue
				}
				argIDs := append([]int64(nil), call.Args...)
				if reg != nil {
					if ks := reg.killArgs(call.GetId()); len(ks) > 0 {
						argIDs = ks
					}
				}
				for _, aid := range argIDs {
					av, ok := call.GetValueById(aid)
					if !ok || av == nil {
						continue
					}
					if idx, throughStar, hit := classifyFreeArg(av, aliases); hit && throughStar {
						freed[idx] = struct{}{}
					}
				}
			}
		}
		if len(freed) == 0 {
			delete(out, fn.GetId())
		}
	})

	for iter := 0; iter < 16; iter++ {
		changed := false
		prog.EachFunction(func(fn *ssa.Function) {
			if fn == nil {
				return
			}
			defer func() { _ = recover() }()
			aliases := collectParamAliases(fn)
			freed := ensure(fn.GetId())
			before := len(freed)
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
					inst = resolveInstruction(inst)
					call, ok := ssa.ToCall(inst)
					if !ok || call == nil || isFreeCall(call, reg) {
						continue
					}
					for _, cid := range resolveCalleeFuncIDs(call) {
						idxs, ok := out[cid]
						if !ok || len(idxs) == 0 {
							continue
						}
						for idx := range idxs {
							if idx < 0 || idx >= len(call.Args) {
								continue
							}
							aid := call.Args[idx]
							if pidx, ok := aliases[aid]; ok {
								freed[pidx] = struct{}{}
								continue
							}
							if av, ok := call.GetValueById(aid); ok && av != nil {
								if i, _, hit := classifyFreeArg(av, aliases); hit {
									freed[i] = struct{}{}
								}
							}
						}
					}
				}
			}
			if len(freed) == 0 {
				delete(out, fn.GetId())
			} else if len(freed) > before {
				changed = true
			}
		})
		if !changed {
			break
		}
	}
	return out
}

// buildParamDerefSummary records which formals each function may dereference
// (member access, bare-* artifact, or passed to another deref-er). Used so
// store-only sinks (held = p) are not reported as UAF call-arg uses.
func buildParamDerefSummary(prog *ssa.Program) map[int64]map[int]struct{} {
	out := make(map[int64]map[int]struct{})
	if prog == nil {
		return out
	}
	ensure := func(fid int64) map[int]struct{} {
		m := out[fid]
		if m == nil {
			m = make(map[int]struct{})
			out[fid] = m
		}
		return m
	}
	markValue := func(aliases map[int64]int, deref map[int]struct{}, v ssa.Value) {
		if v == nil {
			return
		}
		if idx, ok := aliases[v.GetId()]; ok {
			deref[idx] = struct{}{}
		}
		if pid := paramObjectID(v); pid > 0 {
			if idx, ok := aliases[pid]; ok {
				deref[idx] = struct{}{}
			}
		}
	}

	prog.EachFunction(func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		defer func() { _ = recover() }()
		aliases := collectParamAliases(fn)
		if len(aliases) == 0 {
			return
		}
		deref := ensure(fn.GetId())
		for _, bid := range fn.Blocks {
			b, ok := fn.GetBasicBlockByID(bid)
			if !ok || b == nil {
				continue
			}
			for _, iid := range append(append([]int64{}, b.Phis...), b.Insts...) {
				inst, ok := b.GetInstructionById(iid)
				if !ok || inst == nil {
					continue
				}
				inst = resolveInstruction(inst)
				v, ok := inst.(ssa.Value)
				if !ok || v == nil {
					continue
				}
				if v.IsMember() && !isUndefinedValue(v) {
					markValue(aliases, deref, v.GetObject())
				}
				if ptr := starDerefPointer(v); ptr != nil {
					markValue(aliases, deref, ptr)
				}
				if ptr := registeredDerefPointer(v, getReg(prog)); ptr != nil {
					markValue(aliases, deref, ptr)
				}
			}
		}
		if len(deref) == 0 {
			delete(out, fn.GetId())
		}
	})

	for iter := 0; iter < 16; iter++ {
		changed := false
		prog.EachFunction(func(fn *ssa.Function) {
			if fn == nil {
				return
			}
			defer func() { _ = recover() }()
			aliases := collectParamAliases(fn)
			deref := ensure(fn.GetId())
			before := len(deref)
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
					inst = resolveInstruction(inst)
					call, ok := ssa.ToCall(inst)
					if !ok || call == nil {
						continue
					}
					for _, cid := range resolveCalleeFuncIDs(call) {
						idxs, ok := out[cid]
						if !ok {
							continue
						}
						for idx := range idxs {
							if idx < 0 || idx >= len(call.Args) {
								continue
							}
							if av, ok := call.GetValueById(call.Args[idx]); ok {
								markValue(aliases, deref, av)
							}
						}
					}
				}
			}
			if len(deref) == 0 {
				delete(out, fn.GetId())
			} else if len(deref) > before {
				changed = true
			}
		})
		if !changed {
			break
		}
	}
	return out
}

// functionHasNontrivialBody is true when fn has more than an empty entry
// (used to distinguish store-only definitions from extern declarations).
func functionHasNontrivialBody(fn *ssa.Function) bool {
	if fn == nil {
		return false
	}
	n := 0
	for _, bid := range fn.Blocks {
		b, ok := fn.GetBasicBlockByID(bid)
		if !ok || b == nil {
			continue
		}
		n += len(b.Insts) + len(b.Phis)
		if n > 2 {
			return true
		}
	}
	return n > 0
}

// FindUAFUsesRelated returns UAF uses related to the given pointer/alloc values.
// Empty seeds yield no findings (unlike FindUAFUses, which scans the whole program).
func FindUAFUsesRelated(prog *ssa.Program, seeds []ssa.Value) []*Finding {
	if len(seeds) == 0 {
		return nil
	}
	seedIDs := expandLifetimeSeedIDs(seeds)
	if len(seedIDs) == 0 {
		return nil
	}
	all := FindUAFUses(prog)
	var out []*Finding
	reg := getReg(prog)
	for _, f := range all {
		if findingRelatedToSeeds(f, seedIDs, reg) {
			out = append(out, f)
		}
	}
	return out
}

// entryStateFor seeds formal parameters as abstract heap/resource objects
// (Fortify-style rule variables). Object id == parameter value id. Parameters
// start Alive; only an explicit free/kill in this function moves them to Freed.
// Does not assume the caller already freed them (avoids speculative interproc FP).
func entryStateFor(fn *ssa.Function) *pathState {
	st := newPathState()
	if fn == nil {
		return st
	}
	for _, pid := range fn.Params {
		if pid <= 0 {
			continue
		}
		inst, ok := fn.GetInstructionById(pid)
		if !ok || inst == nil {
			continue
		}
		p, ok := ssa.ToParameter(inst)
		if !ok || p == nil || p.IsFreeValue {
			continue
		}
		id := p.GetId()
		if id <= 0 {
			continue
		}
		st.objects[id] = StateAlive
		st.addPointsTo(id, id)
	}
	return st
}

func analyzeFunction(fn *ssa.Function, reg *registry, freeParams, freeDerefParams, derefParams map[int64]map[int]struct{}) []*Finding {
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

	// Worklist dataflow until fixpoint
	changed := true
	for iter := 0; changed && iter < len(blocks)*8+8; iter++ {
		changed = false
		for i, b := range blocks {
			preds := make([]*pathState, 0, len(b.Preds))
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
				changed = true
			}
			st := inState[i].clone()
			_ = transferBlock(b, st, allocs, reg, freeParams, freeDerefParams, derefParams)
			if !equalState(outState[i], st) {
				outState[i] = st
				changed = true
			}
		}
	}

	// Collect findings on stable states
	var findings []*Finding
	for i, b := range blocks {
		st := inState[i].clone()
		findings = append(findings, transferBlock(b, st, allocs, reg, freeParams, freeDerefParams, derefParams)...)
	}

	// Dedup findings within function by use id
	seen := make(map[int64]struct{})
	var out []*Finding
	for _, f := range findings {
		if f == nil || f.Use == nil {
			continue
		}
		if _, ok := seen[f.Use.GetId()]; ok {
			continue
		}
		seen[f.Use.GetId()] = struct{}{}
		out = append(out, f)
	}
	return out
}

func transferBlock(b *ssa.BasicBlock, st *pathState, allocs map[int64]struct{}, reg *registry, freeParams, freeDerefParams, derefParams map[int64]map[int]struct{}) []*Finding {
	var findings []*Finding
	// Phis first
	for _, pid := range b.Phis {
		inst, ok := b.GetInstructionById(pid)
		if !ok || inst == nil {
			continue
		}
		inst = resolveInstruction(inst)
		if v, ok := inst.(ssa.Value); ok {
			handlePhiOrValue(v, st, allocs)
		}
	}

	// Index of each alloc site in this block. Used to suppress false UAF on
	// pointer-construction members that appear *before* the alloc Make is
	// revisited on a loop back-edge (object still Freed from prior iteration).
	allocInstIndex := map[int64]int{}
	for i, iid := range b.Insts {
		inst, ok := b.GetInstructionById(iid)
		if !ok || inst == nil {
			continue
		}
		inst = resolveInstruction(inst)
		if v, ok := inst.(ssa.Value); ok && v != nil {
			if _, isAlloc := allocs[v.GetId()]; isAlloc {
				allocInstIndex[v.GetId()] = i
			}
		}
	}

	for i, iid := range b.Insts {
		inst, ok := b.GetInstructionById(iid)
		if !ok || inst == nil {
			continue
		}
		inst = resolveInstruction(inst)
		if inst == nil {
			continue
		}
		fs := handleInst(inst, st, allocs, reg, freeParams, freeDerefParams, derefParams)
		if len(fs) > 0 && len(allocInstIndex) > 0 {
			fs = filterFindingsBeforeAllocRedef(fs, i, allocInstIndex)
		}
		findings = append(findings, fs...)
	}
	return findings
}

// filterFindingsBeforeAllocRedef drops UAF hits whose freed object is an alloc
// defined later in the same block (construction of that alloc, not a real use).
func filterFindingsBeforeAllocRedef(fs []*Finding, instIndex int, allocInstIndex map[int64]int) []*Finding {
	if len(fs) == 0 {
		return fs
	}
	out := fs[:0]
	for _, f := range fs {
		if f == nil {
			continue
		}
		if f.Kind != KindUAF {
			out = append(out, f)
			continue
		}
		if idx, ok := allocInstIndex[f.FreedObj]; ok && instIndex < idx {
			continue
		}
		// Use site itself may be a construction member of a later alloc.
		if f.Use != nil && f.Use.IsMember() {
			if obj := f.Use.GetObject(); obj != nil {
				if idx, ok := allocInstIndex[obj.GetId()]; ok && instIndex < idx {
					continue
				}
			}
		}
		out = append(out, f)
	}
	return out
}

func handlePhiOrValue(v ssa.Value, st *pathState, allocs map[int64]struct{}) {
	if v == nil {
		return
	}
	if _, ok := allocs[v.GetId()]; ok {
		st.objects[v.GetId()] = StateAlive
		st.addPointsTo(v.GetId(), v.GetId())
		return
	}
	phi, ok := ssa.ToPhi(v)
	if !ok || phi == nil {
		return
	}
	merged := make(map[int64]struct{})
	for _, e := range phi.GetValues() {
		if e == nil {
			continue
		}
		for o := range st.objsOf(e.GetId()) {
			merged[o] = struct{}{}
		}
		if _, ok := allocs[e.GetId()]; ok {
			merged[e.GetId()] = struct{}{}
		}
	}
	st.setPointsTo(v.GetId(), merged)
}

func handleInst(inst ssa.Instruction, st *pathState, allocs map[int64]struct{}, reg *registry, freeParams, freeDerefParams, derefParams map[int64]map[int]struct{}) []*Finding {
	var findings []*Finding
	v, isVal := inst.(ssa.Value)
	if !isVal || v == nil {
		return nil
	}

	// Allocation site: (re)birth of the heap object. On loop back-edges the
	// same Make is revisited; revive it and drop Freed marks on its
	// construction members so pointer setup is not reported as UAF.
	if _, ok := allocs[v.GetId()]; ok {
		st.objects[v.GetId()] = StateAlive
		delete(st.killedBy, v.GetId())
		st.addPointsTo(v.GetId(), v.GetId())
		for _, rid := range pointerRelatedIDs(v) {
			if rid == v.GetId() {
				continue
			}
			if st.objects[rid] == StateFreed {
				delete(st.objects, rid)
				delete(st.killedBy, rid)
			}
		}
		return nil
	}

	// Null const clears points-to for this value id
	if isNullish(v) {
		st.setPointsTo(v.GetId(), nil)
		return nil
	}

	if call, ok := ssa.ToCall(v); ok && call != nil {
		return handleCall(call, st, allocs, reg, freeParams, freeDerefParams, derefParams)
	}

	if phi, ok := ssa.ToPhi(v); ok && phi != nil {
		handlePhiOrValue(v, st, allocs)
		return nil
	}

	// Propagate points-to through casts / unary / simple copies
	propagateFromOperands(v, st, allocs)

	// Member access: object relationship may not appear in GetValues()
	if v.IsMember() && !isUndefinedValue(v) {
		if obj := v.GetObject(); obj != nil {
			for o := range st.objsOf(obj.GetId()) {
				st.addPointsTo(v.GetId(), o)
			}
			if st.objects[obj.GetId()] == StateFreed {
				st.addPointsTo(v.GetId(), obj.GetId())
			}
			if _, ok := allocs[obj.GetId()]; ok {
				st.addPointsTo(v.GetId(), obj.GetId())
			}
			// parameter[i].@value / @pointer → formal parameter abstract object
			if pid := paramObjectID(obj); pid > 0 {
				st.addPointsTo(obj.GetId(), pid)
				st.addPointsTo(v.GetId(), pid)
			}
		}
	}

	// Bare-* artifact may carry points-to of the pointer operand.
	if ptr := starDerefPointer(v); ptr != nil {
		for o := range st.objsOf(ptr.GetId()) {
			st.addPointsTo(v.GetId(), o)
		}
		if pid := paramObjectID(ptr); pid > 0 {
			st.addPointsTo(v.GetId(), pid)
		}
	}
	if ptr := registeredDerefPointer(v, reg); ptr != nil {
		for o := range st.objsOf(ptr.GetId()) {
			st.addPointsTo(v.GetId(), o)
		}
		if pid := paramObjectID(ptr); pid > 0 {
			st.addPointsTo(v.GetId(), pid)
		}
	}

	// Check operands / member object for UAF use
	findings = append(findings, checkUses(v, st)...)
	return findings
}

func propagateFromOperands(v ssa.Value, st *pathState, allocs map[int64]struct{}) {
	if v == nil || !v.HasValues() {
		return
	}
	merged := make(map[int64]struct{})
	for _, op := range v.GetValues() {
		if op == nil {
			continue
		}
		for o := range st.objsOf(op.GetId()) {
			merged[o] = struct{}{}
		}
		if _, ok := allocs[op.GetId()]; ok {
			merged[op.GetId()] = struct{}{}
		}
	}
	if len(merged) > 0 {
		st.setPointsTo(v.GetId(), merged)
	}
}

func handleCall(call *ssa.Call, st *pathState, allocs map[int64]struct{}, reg *registry, freeParams, freeDerefParams, derefParams map[int64]map[int]struct{}) []*Finding {
	var findings []*Finding
	isFree := isFreeCall(call, reg)

	argIDs := append([]int64(nil), call.Args...)
	seenArgID := map[int64]struct{}{}
	for _, id := range argIDs {
		seenArgID[id] = struct{}{}
	}
	if reg != nil {
		for _, id := range reg.killArgs(call.GetId()) {
			if id <= 0 {
				continue
			}
			if _, ok := seenArgID[id]; ok {
				continue
			}
			// Ignore kill-registry ids that do not resolve to a value; they
			// create ghost objects and spurious double-free on loops.
			if av, ok := call.GetValueById(id); !ok || av == nil {
				continue
			}
			argIDs = append(argIDs, id)
			seenArgID[id] = struct{}{}
		}
	}

	applyKill := func(aids []int64) {
		for _, aid := range aids {
			if aid <= 0 {
				continue
			}
			// Freed pointer value is itself an object identity.
			st.addPointsTo(aid, aid)
			objs := make(map[int64]struct{})
			for o := range st.objsOf(aid) {
				objs[o] = struct{}{}
			}
			objs[aid] = struct{}{}
			if _, ok := allocs[aid]; ok {
				objs[aid] = struct{}{}
			}
			if av, ok := call.GetValueById(aid); ok && av != nil {
				if _, ok := allocs[av.GetId()]; ok {
					objs[av.GetId()] = struct{}{}
				}
			}
			for oid := range objs {
				if st.objects[oid] == StateFreed {
					findings = append(findings, &Finding{
						Use:      call,
						FreedObj: oid,
						FreeCall: st.killedBy[oid],
						Kind:     KindDoubleFree,
					})
					continue
				}
				st.objects[oid] = StateFreed
				st.killedBy[oid] = call
			}
		}
	}

	if isFree {
		applyKill(argIDs)
		return findings
	}

	// realloc invalidates the old pointer (first arg); result is a fresh alloc.
	if strings.EqualFold(callMethodName(call), "realloc") && len(call.Args) > 0 {
		findings = append(findings, checkUses(call, st)...)
		applyKill([]int64{call.Args[0]})
		return findings
	}

	// Interprocedural: callee frees formal(s) and/or *formal(s).
	calleeIDs := resolveCalleeFuncIDs(call)
	var killArgs []int64
	seenArg := make(map[int64]struct{})
	addKill := func(aid int64) {
		if aid <= 0 {
			return
		}
		if _, ok := seenArg[aid]; ok {
			return
		}
		seenArg[aid] = struct{}{}
		killArgs = append(killArgs, aid)
	}
	if freeParams != nil {
		for _, cid := range calleeIDs {
			idxs, ok := freeParams[cid]
			if !ok || len(idxs) == 0 {
				continue
			}
			for idx := range idxs {
				if idx >= 0 && idx < len(call.Args) {
					addKill(call.Args[idx])
				}
			}
		}
	}
	if freeDerefParams != nil {
		for _, cid := range calleeIDs {
			idxs, ok := freeDerefParams[cid]
			if !ok || len(idxs) == 0 {
				continue
			}
			for idx := range idxs {
				if idx < 0 || idx >= len(call.Args) {
					continue
				}
				av, _ := call.GetValueById(call.Args[idx])
				for _, pid := range pointerPointeeIDs(av) {
					addKill(pid)
				}
				addHeapMembers := func(obj ssa.Value) {
					if obj == nil {
						return
					}
					for _, m := range obj.GetAllMember() {
						if m == nil {
							continue
						}
						if isHeapAllocValue(m) {
							addKill(m.GetId())
							continue
						}
						if allocs != nil {
							if _, ok := allocs[m.GetId()]; ok {
								addKill(m.GetId())
							}
						}
					}
				}
				addHeapMembers(av)
				addHeapMembers(call)
			}
		}
	}
	if len(killArgs) > 0 {
		findings = append(findings, checkUses(call, st)...)
		applyKill(killArgs)
		return findings
	}

	// Non-free call: using freed pointer as argument is UAF — unless every
	// resolved callee has a nontrivial body and does not dereference that formal
	// (store-only sinks like `held = p`).
	if shouldCheckCallArgUAF(call, derefParams) {
		findings = append(findings, checkUses(call, st)...)
	}
	return findings
}

func shouldCheckCallArgUAF(call *ssa.Call, derefParams map[int64]map[int]struct{}) bool {
	if call == nil {
		return true
	}
	calleeIDs := resolveCalleeFuncIDs(call)
	if len(calleeIDs) == 0 {
		return true // extern / unresolved → conservative
	}
	prog := call.GetProgram()
	for _, cid := range calleeIDs {
		var fn *ssa.Function
		if prog != nil {
			prog.EachFunction(func(f *ssa.Function) {
				if f != nil && f.GetId() == cid {
					fn = f
				}
			})
		}
		if fn == nil || !functionHasNontrivialBody(fn) {
			return true
		}
		if derefParams != nil {
			if idxs, ok := derefParams[cid]; ok && len(idxs) > 0 {
				return true
			}
		}
	}
	return false
}

func resolveCalleeFuncIDs(call *ssa.Call) []int64 {
	if call == nil {
		return nil
	}
	method, ok := call.GetValueById(call.Method)
	if !ok || method == nil {
		return nil
	}
	if fn, ok := ssa.ToFunction(method); ok && fn != nil {
		return []int64{fn.GetId()}
	}
	name := strings.TrimPrefix(method.GetName(), "Function-")
	if name == "" {
		name = method.GetVerboseName()
	}
	if name == "" {
		return nil
	}
	prog := call.GetProgram()
	if prog == nil {
		return nil
	}
	var ids []int64
	prog.EachFunction(func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		if fn.GetName() == name || strings.HasSuffix(fn.GetName(), name) {
			ids = append(ids, fn.GetId())
		}
	})
	return ids
}

func checkUses(v ssa.Value, st *pathState) []*Finding {
	if v == nil {
		return nil
	}
	var findings []*Finding
	seenObj := make(map[int64]struct{})
	report := func(oid int64) {
		if st.objects[oid] != StateFreed {
			return
		}
		if _, ok := seenObj[oid]; ok {
			return
		}
		seenObj[oid] = struct{}{}
		findings = append(findings, &Finding{
			Use:      v,
			FreedObj: oid,
			FreeCall: st.killedBy[oid],
			Kind:     KindUAF,
		})
	}

	if v.HasValues() {
		for _, op := range v.GetValues() {
			if op == nil {
				continue
			}
			oidList := st.objsOf(op.GetId())
			if len(oidList) == 0 && st.objects[op.GetId()] == StateFreed {
				report(op.GetId())
			}
			for oid := range oidList {
				report(oid)
			}
			if pid := paramObjectID(op); pid > 0 {
				report(pid)
			}
		}
	}

	if ptr := starDerefPointer(v); ptr != nil {
		for oid := range st.objsOf(ptr.GetId()) {
			report(oid)
		}
		if st.objects[ptr.GetId()] == StateFreed {
			report(ptr.GetId())
		}
		if pid := paramObjectID(ptr); pid > 0 {
			report(pid)
		}
	}

	if ptr := registeredDerefPointer(v, getReg(v.GetProgram())); ptr != nil {
		for oid := range st.objsOf(ptr.GetId()) {
			report(oid)
		}
		if st.objects[ptr.GetId()] == StateFreed {
			report(ptr.GetId())
		}
		if pid := paramObjectID(ptr); pid > 0 {
			report(pid)
		}
	}

	if v.IsMember() && !isUndefinedValue(v) {
		if obj := v.GetObject(); obj != nil {
			for oid := range st.objsOf(obj.GetId()) {
				report(oid)
			}
			if st.objects[obj.GetId()] == StateFreed {
				report(obj.GetId())
			}
			if pid := paramObjectID(obj); pid > 0 {
				report(pid)
			}
		}
	}

	// Standalone use of a freed heap value itself
	if st.objects[v.GetId()] == StateFreed && !v.HasValues() && !v.IsMember() {
		report(v.GetId())
	}
	if pid := paramObjectID(v); pid > 0 {
		report(pid)
	}
	return findings
}
