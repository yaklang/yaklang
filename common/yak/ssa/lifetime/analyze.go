package lifetime

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Finding is a use-after-free (or double-free) site.
type Finding struct {
	Use      ssa.Value // instruction / value reported as UAF use
	FreedObj int64     // heap object id
	FreeCall ssa.Value // optional free call that killed the object on this path
	Kind     string    // "uaf" | "double-free"
}

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

func isNullish(v ssa.Value) bool {
	if utils.IsNil(v) {
		return true
	}
	c, ok := ssa.ToConstInst(v)
	if !ok || c == nil {
		return false
	}
	if c.Const != nil && c.Const.GetRawValue() == nil {
		return true
	}
	s := strings.ToLower(strings.TrimSpace(c.String()))
	return s == "0" || s == "null" || s == "nil" || s == "<nil>" || s == "nullptr"
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

// FindUAFUses analyzes prog and returns UAF / double-free use sites.
func FindUAFUses(prog *ssa.Program) []*Finding {
	if prog == nil {
		return nil
	}
	reg := getReg(prog)
	// Callee summaries: function id -> formal param indexes that are freed.
	freeParams := buildFreeParamSummary(prog)
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
		for _, f := range analyzeFunction(fn, reg, freeParams) {
			add(f)
		}
	})
	return findings
}

// buildFreeParamSummary records which formal parameters each function frees
// (directly via free(param) / RegisterKill). Used for simple interprocedural UAF.
func buildFreeParamSummary(prog *ssa.Program) map[int64]map[int]struct{} {
	out := make(map[int64]map[int]struct{})
	if prog == nil {
		return out
	}
	reg := getReg(prog)
	prog.EachFunction(func(fn *ssa.Function) {
		if fn == nil {
			return
		}
		defer func() { _ = recover() }()
		paramIndexByID := make(map[int64]int)
		for i, pid := range fn.Params {
			paramIndexByID[pid] = i
		}
		freed := make(map[int]struct{})
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
		if len(freed) > 0 {
			out[fn.GetId()] = freed
		}
	})
	return out
}

// FindUAFUsesRelated returns UAF uses related to the given pointer/alloc values.
func FindUAFUsesRelated(prog *ssa.Program, seeds []ssa.Value) []*Finding {
	all := FindUAFUses(prog)
	if len(seeds) == 0 {
		return all
	}
	seedObjs := make(map[int64]struct{})
	reg := getReg(prog)
	allocs := map[int64]struct{}{}
	if reg != nil {
		allocs = reg.snapshotAlloc()
	}
	for _, s := range seeds {
		if s == nil {
			continue
		}
		id := s.GetId()
		seedObjs[id] = struct{}{}
		if _, ok := allocs[id]; ok {
			seedObjs[id] = struct{}{}
		}
		// If seed is free() call, include its args' objects.
		if c, ok := ssa.ToCall(s); ok && c != nil {
			for _, aid := range c.Args {
				seedObjs[aid] = struct{}{}
			}
		}
	}
	var out []*Finding
	for _, f := range all {
		if _, ok := seedObjs[f.FreedObj]; ok {
			out = append(out, f)
			continue
		}
		if f.FreeCall != nil {
			if _, ok := seedObjs[f.FreeCall.GetId()]; ok {
				out = append(out, f)
			}
		}
	}
	return out
}

func analyzeFunction(fn *ssa.Function, reg *registry, freeParams map[int64]map[int]struct{}) []*Finding {
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
			if b.GetId() == entryID || len(b.Preds) == 0 {
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
			_ = transferBlock(b, st, allocs, reg, freeParams)
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
		findings = append(findings, transferBlock(b, st, allocs, reg, freeParams)...)
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

func discoverAllocs(fn *ssa.Function, reg *registry) map[int64]struct{} {
	allocs := map[int64]struct{}{}
	if reg != nil {
		for id := range reg.snapshotAlloc() {
			allocs[id] = struct{}{}
		}
	}
	mark := func(v ssa.Value) {
		if v == nil || v.GetId() <= 0 {
			return
		}
		if isHeapAllocValue(v) {
			allocs[v.GetId()] = struct{}{}
		}
	}
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
			if v, ok := inst.(ssa.Value); ok {
				mark(v)
			} else if resolved := resolveInstruction(inst); resolved != nil {
				if v, ok := resolved.(ssa.Value); ok {
					mark(v)
				}
			}
		}
	}
	return allocs
}

func isHeapAllocValue(v ssa.Value) bool {
	if v == nil {
		return false
	}
	if strings.HasPrefix(v.GetVerboseName(), HeapAllocVerbosePrefix) {
		return true
	}
	if strings.HasPrefix(v.GetName(), "$heap_") {
		return true
	}
	// Pointer wrapper from emitHeapPointer: @pointer holds "$heap_N#id"
	if t := v.GetType(); t != nil && t.GetTypeKind() == ssa.PointerKind {
		if members := v.GetMembersByKeyString("@pointer"); len(members) > 0 {
			for _, m := range members {
				if m == nil {
					continue
				}
				s := m.String()
				if strings.Contains(s, "$heap_") || strings.Contains(s, HeapAllocVerbosePrefix) {
					return true
				}
			}
		}
		if p, ok := v.GetStringMember("@pointer"); ok && p != nil {
			s := p.String()
			if strings.Contains(s, "$heap_") || strings.Contains(s, HeapAllocVerbosePrefix) {
				return true
			}
		}
	}
	return false
}

// pointerRelatedIDs returns value ids tied to a pointer wrapper (@pointer / @value members).
func pointerRelatedIDs(v ssa.Value) []int64 {
	if v == nil {
		return nil
	}
	var ids []int64
	seen := map[int64]struct{}{}
	add := func(x ssa.Value) {
		if x == nil || x.GetId() <= 0 {
			return
		}
		if _, ok := seen[x.GetId()]; ok {
			return
		}
		seen[x.GetId()] = struct{}{}
		ids = append(ids, x.GetId())
	}
	add(v)
	for _, key := range []string{"@pointer", "@value"} {
		for _, m := range v.GetMembersByKeyString(key) {
			add(m)
		}
		if m, ok := v.GetStringMember(key); ok {
			add(m)
		}
	}
	return ids
}

func resolveInstruction(inst ssa.Instruction) ssa.Instruction {
	if inst == nil {
		return nil
	}
	defer func() { _ = recover() }()
	if lz, ok := ssa.ToLazyInstruction(inst); ok && lz != nil {
		if self := lz.Self(); self != nil {
			return self
		}
	}
	return inst
}

func transferBlock(b *ssa.BasicBlock, st *pathState, allocs map[int64]struct{}, reg *registry, freeParams map[int64]map[int]struct{}) []*Finding {
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
		fs := handleInst(inst, st, allocs, reg, freeParams)
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
		if f.Kind != "uaf" {
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

func handleInst(inst ssa.Instruction, st *pathState, allocs map[int64]struct{}, reg *registry, freeParams map[int64]map[int]struct{}) []*Finding {
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
		return handleCall(call, st, allocs, reg, freeParams)
	}

	if phi, ok := ssa.ToPhi(v); ok && phi != nil {
		handlePhiOrValue(v, st, allocs)
		return nil
	}

	// Propagate points-to through casts / unary / simple copies
	propagateFromOperands(v, st, allocs)

	// Member access: object relationship may not appear in GetValues()
	if v.IsMember() {
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

func handleCall(call *ssa.Call, st *pathState, allocs map[int64]struct{}, reg *registry, freeParams map[int64]map[int]struct{}) []*Finding {
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
						Kind:     "double-free",
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

	// Interprocedural: callee frees formal parameter(s) → kill matching actual args.
	if freeParams != nil {
		calleeIDs := resolveCalleeFuncIDs(call)
		var killArgs []int64
		seenArg := make(map[int64]struct{})
		for _, cid := range calleeIDs {
			idxs, ok := freeParams[cid]
			if !ok || len(idxs) == 0 {
				continue
			}
			for idx := range idxs {
				if idx >= 0 && idx < len(call.Args) {
					aid := call.Args[idx]
					if _, ok := seenArg[aid]; ok {
						continue
					}
					seenArg[aid] = struct{}{}
					killArgs = append(killArgs, aid)
				}
			}
		}
		if len(killArgs) > 0 {
			findings = append(findings, checkUses(call, st)...)
			applyKill(killArgs)
			return findings
		}
	}

	// Non-free call: using freed pointer as argument is UAF
	findings = append(findings, checkUses(call, st)...)
	return findings
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
			Kind:     "uaf",
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
		}
	}

	if v.IsMember() {
		if obj := v.GetObject(); obj != nil {
			for oid := range st.objsOf(obj.GetId()) {
				report(oid)
			}
			if st.objects[obj.GetId()] == StateFreed {
				report(obj.GetId())
			}
		}
	}

	// Standalone use of a freed heap value itself
	if st.objects[v.GetId()] == StateFreed && !v.HasValues() && !v.IsMember() {
		report(v.GetId())
	}
	return findings
}
