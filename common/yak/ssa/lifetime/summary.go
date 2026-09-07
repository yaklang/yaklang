package lifetime

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// calleeSummaries aggregates interprocedural formal-parameter effects:
// which formals are freed, freed through *, or dereferenced.
type calleeSummaries struct {
	freeParams      map[int64]map[int]struct{}
	freeDerefParams map[int64]map[int]struct{}
	derefParams     map[int64]map[int]struct{}
}

func emptyCalleeSummaries() *calleeSummaries {
	return &calleeSummaries{
		freeParams:      make(map[int64]map[int]struct{}),
		freeDerefParams: make(map[int64]map[int]struct{}),
		derefParams:     make(map[int64]map[int]struct{}),
	}
}

func ensureIdxMap(m map[int64]map[int]struct{}, fid int64) map[int]struct{} {
	dst := m[fid]
	if dst == nil {
		dst = make(map[int]struct{})
		m[fid] = dst
	}
	return dst
}

func pruneEmpty(m map[int64]map[int]struct{}, fid int64) {
	if len(m[fid]) == 0 {
		delete(m, fid)
	}
}

// buildCalleeSummaries scans each function once for direct free / free-* /
// deref effects, then runs a joint fixpoint (max 16 rounds) for propagation.
func buildCalleeSummaries(prog *ssa.Program, reg *registry) *calleeSummaries {
	out := emptyCalleeSummaries()
	if prog == nil {
		return out
	}

	markDeref := func(aliases map[int64]int, deref map[int]struct{}, v ssa.Value) {
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

	// Pass 1: direct effects per function (single instruction scan).
	prog.EachFunction(func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		defer func() { _ = recover() }()
		fid := fn.GetId()
		aliases := collectParamAliases(fn)
		freed := ensureIdxMap(out.freeParams, fid)
		freedStar := ensureIdxMap(out.freeDerefParams, fid)
		deref := ensureIdxMap(out.derefParams, fid)

		for _, bid := range fn.Blocks {
			b, ok := fn.GetBasicBlockByID(bid)
			if !ok || b == nil {
				continue
			}
			// Phis + Insts for deref; Insts only for free (matches old builders).
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
				if len(aliases) > 0 {
					if v.IsMember() && !isUndefinedValue(v) {
						markDeref(aliases, deref, v.GetObject())
					}
					if ptr := starDerefPointer(v); ptr != nil {
						markDeref(aliases, deref, ptr)
					}
					if ptr := registeredDerefPointer(v, reg); ptr != nil {
						markDeref(aliases, deref, ptr)
					}
				}
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
					if idx, ok := aliases[aid]; ok {
						freed[idx] = struct{}{}
					} else if av, ok := call.GetValueById(aid); ok && av != nil {
						if p, ok := ssa.ToParameter(av); ok && p != nil && !p.IsFreeValue {
							freed[p.FormalParameterIndex] = struct{}{}
						}
					}
					av, ok := call.GetValueById(aid)
					if !ok || av == nil {
						continue
					}
					if idx, throughStar, hit := classifyFreeArg(av, aliases); hit && throughStar {
						freedStar[idx] = struct{}{}
					}
				}
			}
		}
		pruneEmpty(out.freeParams, fid)
		pruneEmpty(out.freeDerefParams, fid)
		pruneEmpty(out.derefParams, fid)
	})

	// Pass 2: joint fixpoint — free / free-* / deref propagate through calls.
	for iter := 0; iter < 16; iter++ {
		changed := false
		prog.EachFunction(func(fn *ssa.Function) {
			if fn == nil {
				return
			}
			defer func() { _ = recover() }()
			fid := fn.GetId()
			aliases := collectParamAliases(fn)
			freed := ensureIdxMap(out.freeParams, fid)
			freedStar := ensureIdxMap(out.freeDerefParams, fid)
			deref := ensureIdxMap(out.derefParams, fid)
			beforeFree := len(freed)
			beforeStar := len(freedStar)
			beforeDeref := len(deref)

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
					isFree := isFreeCall(call, reg)
					for _, cid := range resolveCalleeFuncIDs(call) {
						if !isFree {
							if idxs, ok := out.freeParams[cid]; ok {
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
										if p, ok := ssa.ToParameter(av); ok && p != nil && !p.IsFreeValue {
											freed[p.FormalParameterIndex] = struct{}{}
										}
									}
								}
							}
							if idxs, ok := out.freeDerefParams[cid]; ok {
								for idx := range idxs {
									if idx < 0 || idx >= len(call.Args) {
										continue
									}
									aid := call.Args[idx]
									if pidx, ok := aliases[aid]; ok {
										freedStar[pidx] = struct{}{}
										continue
									}
									if av, ok := call.GetValueById(aid); ok && av != nil {
										if i, _, hit := classifyFreeArg(av, aliases); hit {
											freedStar[i] = struct{}{}
										}
									}
								}
							}
						}
						if idxs, ok := out.derefParams[cid]; ok {
							for idx := range idxs {
								if idx < 0 || idx >= len(call.Args) {
									continue
								}
								if av, ok := call.GetValueById(call.Args[idx]); ok {
									markDeref(aliases, deref, av)
								}
							}
						}
					}
				}
			}
			pruneEmpty(out.freeParams, fid)
			pruneEmpty(out.freeDerefParams, fid)
			pruneEmpty(out.derefParams, fid)
			if len(freed) > beforeFree || len(freedStar) > beforeStar || len(deref) > beforeDeref {
				changed = true
			}
		})
		if !changed {
			break
		}
	}
	return out
}
