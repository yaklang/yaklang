package lifetime

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// NullState is the emptiness of a pointer-typed SSA value on a CFG path.
type NullState int

const (
	NullUnknown NullState = iota // not yet constrained (e.g. formal param)
	NullNonNull                  // definitely non-null (e.g. malloc result)
	NullIsNull                   // definitely null (0 / NULL / nullptr)
	NullMaybe                    // may be null after path join
)

type npdState struct {
	nullness map[int64]NullState
}

func newNPDState() *npdState {
	return &npdState{nullness: make(map[int64]NullState)}
}

func (s *npdState) clone() *npdState {
	n := newNPDState()
	for k, v := range s.nullness {
		n.nullness[k] = v
	}
	return n
}

func (s *npdState) get(id int64) NullState {
	if s == nil || id <= 0 {
		return NullUnknown
	}
	if st, ok := s.nullness[id]; ok {
		return st
	}
	return NullUnknown
}

func (s *npdState) set(id int64, st NullState) {
	if s == nil || id <= 0 {
		return
	}
	if st == NullUnknown {
		delete(s.nullness, id)
		return
	}
	s.nullness[id] = st
}

func mergeNull(a, b NullState) NullState {
	if a == NullUnknown {
		return b
	}
	if b == NullUnknown {
		return a
	}
	if a == b {
		return a
	}
	// Null vs NonNull (or already Maybe) → Maybe
	return NullMaybe
}

func mergeNPDStates(states []*npdState) *npdState {
	out := newNPDState()
	if len(states) == 0 {
		return out
	}
	ids := map[int64]struct{}{}
	for _, s := range states {
		if s == nil {
			continue
		}
		for id := range s.nullness {
			ids[id] = struct{}{}
		}
	}
	for id := range ids {
		acc := NullUnknown
		for _, s := range states {
			if s == nil {
				continue
			}
			acc = mergeNull(acc, s.get(id))
		}
		out.set(id, acc)
	}
	return out
}

func equalNPDState(a, b *npdState) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.nullness) != len(b.nullness) {
		return false
	}
	for k, v := range a.nullness {
		if b.nullness[k] != v {
			return false
		}
	}
	return true
}

func isDangerousNull(st NullState) bool {
	return st == NullIsNull || st == NullMaybe
}

// FindNPDUses analyzes prog for null-pointer dereferences (independent of UAF).
func FindNPDUses(prog *ssa.Program) []*Finding {
	if prog == nil {
		return nil
	}
	reg := getReg(prog)
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
		for _, f := range analyzeFunctionNPD(fn, reg) {
			add(f)
		}
	})
	return findings
}

// FindNPDUsesRelated filters NPD findings related to seed pointer values.
func FindNPDUsesRelated(prog *ssa.Program, seeds []ssa.Value) []*Finding {
	all := FindNPDUses(prog)
	if len(seeds) == 0 {
		return all
	}
	seedIDs := make(map[int64]struct{})
	for _, s := range seeds {
		if s == nil || s.GetId() <= 0 {
			continue
		}
		seedIDs[s.GetId()] = struct{}{}
	}
	var out []*Finding
	for _, f := range all {
		if _, ok := seedIDs[f.FreedObj]; ok {
			out = append(out, f)
		}
	}
	return out
}

func analyzeFunctionNPD(fn *ssa.Function, reg *registry) []*Finding {
	if fn == nil || len(fn.Blocks) == 0 {
		return nil
	}
	allocs := discoverAllocs(fn, reg)

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

	inState := make([]*npdState, len(blocks))
	outState := make([]*npdState, len(blocks))
	for i := range blocks {
		inState[i] = newNPDState()
		outState[i] = newNPDState()
	}

	entryID := fn.EnterBlock
	if entryID <= 0 && len(fn.Blocks) > 0 {
		entryID = fn.Blocks[0]
	}
	// Params stay Unknown — do not assume they are null (conservative).

	changed := true
	for iter := 0; changed && iter < len(blocks)*8+8; iter++ {
		changed = false
		for i, b := range blocks {
			preds := make([]*npdState, 0, len(b.Preds)+1)
			if b.GetId() == entryID || len(b.Preds) == 0 {
				preds = append(preds, newNPDState())
			}
			for _, pid := range b.Preds {
				if idx, ok := blockIndex[pid]; ok {
					preds = append(preds, outState[idx])
				}
			}
			merged := mergeNPDStates(preds)
			if !equalNPDState(inState[i], merged) {
				inState[i] = merged
				changed = true
			}
			st := inState[i].clone()
			_ = transferBlockNPD(b, st, allocs)
			if !equalNPDState(outState[i], st) {
				outState[i] = st
				changed = true
			}
		}
	}

	var findings []*Finding
	for i, b := range blocks {
		st := inState[i].clone()
		findings = append(findings, transferBlockNPD(b, st, allocs)...)
	}
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

func transferBlockNPD(b *ssa.BasicBlock, st *npdState, allocs map[int64]struct{}) []*Finding {
	var findings []*Finding
	for _, pid := range b.Phis {
		inst, ok := b.GetInstructionById(pid)
		if !ok || inst == nil {
			continue
		}
		inst = resolveInstruction(inst)
		if v, ok := inst.(ssa.Value); ok {
			handlePhiNPD(v, st, allocs)
		}
	}
	for _, iid := range b.Insts {
		inst, ok := b.GetInstructionById(iid)
		if !ok || inst == nil {
			continue
		}
		inst = resolveInstruction(inst)
		if inst == nil {
			continue
		}
		findings = append(findings, handleInstNPD(inst, st, allocs)...)
	}
	return findings
}

func handlePhiNPD(v ssa.Value, st *npdState, allocs map[int64]struct{}) {
	if v == nil {
		return
	}
	if _, ok := allocs[v.GetId()]; ok {
		st.set(v.GetId(), NullNonNull)
		return
	}
	phi, ok := ssa.ToPhi(v)
	if !ok || phi == nil {
		return
	}
	acc := NullUnknown
	for _, e := range phi.GetValues() {
		if e == nil {
			continue
		}
		acc = mergeNull(acc, nullnessOf(e, st, allocs))
	}
	st.set(v.GetId(), acc)
}

func nullnessOf(v ssa.Value, st *npdState, allocs map[int64]struct{}) NullState {
	if v == nil {
		return NullUnknown
	}
	id := v.GetId()
	if _, ok := allocs[id]; ok {
		return NullNonNull
	}
	if isHeapAllocValue(v) {
		return NullNonNull
	}
	if isNullish(v) {
		return NullIsNull
	}
	if st != nil {
		if s := st.get(id); s != NullUnknown {
			return s
		}
	}
	return NullUnknown
}

func handleInstNPD(inst ssa.Instruction, st *npdState, allocs map[int64]struct{}) []*Finding {
	v, isVal := inst.(ssa.Value)
	if !isVal || v == nil {
		return nil
	}
	id := v.GetId()

	if _, ok := allocs[id]; ok || isHeapAllocValue(v) {
		st.set(id, NullNonNull)
		return nil
	}

	if isNullish(v) {
		st.set(id, NullIsNull)
		// Null constants themselves are not dereferences.
		return nil
	}

	if call, ok := ssa.ToCall(v); ok && call != nil {
		// Calls are not NPD uses in phase 1 (unlike UAF). Still propagate return if needed.
		_ = call
		return nil
	}

	if phi, ok := ssa.ToPhi(v); ok && phi != nil {
		handlePhiNPD(v, st, allocs)
		return nil
	}

	// Propagate nullness from operands (copies / casts).
	if v.HasValues() {
		acc := NullUnknown
		for _, op := range v.GetValues() {
			if op == nil {
				continue
			}
			acc = mergeNull(acc, nullnessOf(op, st, allocs))
		}
		if acc != NullUnknown {
			st.set(id, acc)
		}
	}

	// Member access: treating member write/read of a null pointer as NPD.
	var findings []*Finding
	if v.IsMember() {
		if obj := v.GetObject(); obj != nil {
			oids := []int64{obj.GetId()}
			if pid := paramObjectID(obj); pid > 0 {
				oids = append(oids, pid)
			}
			for _, oid := range oids {
				stObj := st.get(oid)
				if stObj == NullUnknown {
					stObj = nullnessOf(obj, st, allocs)
				}
				if isDangerousNull(stObj) {
					findings = append(findings, &Finding{
						Use:      v,
						FreedObj: oid,
						Kind:     KindNPD,
					})
					break
				}
			}
			// Member inherits object's nullness for further aliasing (weak).
			if cur := st.get(obj.GetId()); cur != NullUnknown {
				st.set(id, cur)
			}
		}
	}
	return findings
}
