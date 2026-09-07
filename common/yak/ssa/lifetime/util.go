package lifetime

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Finding is a lifetime / nullness violation site.
// Double-free is a UAF subtype. NPD and Leak are independent.
// For KindNPD, FreedObj holds the pointer value id that was Null/MaybeNull.
// For KindLeak, Use is the leaked alloc; FreedObj is the object id.
type Finding struct {
	Use      ssa.Value // violating use (deref / 2nd free / leaked alloc / …)
	FreedObj int64     // heap/abstract object id; for NPD: pointer value id
	FreeCall ssa.Value // optional prior free (UAF/double-free only)
	Kind     string    // KindUAF | KindDoubleFree | KindNPD | KindLeak
}

// Finding kinds returned by FindUAFUses / FindNPDUses / FindMemLeaks.
const (
	KindUAF        = "uaf"
	KindDoubleFree = "double-free" // UAF subtype: free after free
	KindNPD        = "npd"         // null pointer dereference (independent of UAF)
	KindLeak       = "mem-leak"    // heap alloc still Alive at exit (may-leak)
)

func isPointerish(v ssa.Value) bool {
	if v == nil {
		return false
	}
	if _, ok := ssa.ToConstInst(v); ok {
		return false
	}
	if isHeapAllocValue(v) {
		return true
	}
	if t := v.GetType(); t != nil && t.GetTypeKind() == ssa.PointerKind {
		return true
	}
	if _, ok := ssa.ToParameter(v); ok {
		return true
	}
	if _, ok := ssa.ToPhi(v); ok {
		return true
	}
	return false
}

// expandLifetimeSeedIDs collects SSA ids that identify the same pointer/object
// as the SyntaxFlow seeds: the value itself, pointer copies/phis, and free() args.
// Constants (malloc size, store RHS) are excluded so two pointers cannot collide
// via a shared interned integer.
func expandLifetimeSeedIDs(seeds []ssa.Value) map[int64]struct{} {
	ids := make(map[int64]struct{})
	var walk func(ssa.Value, int, bool)
	walk = func(v ssa.Value, depth int, isSeed bool) {
		if v == nil || depth > 24 {
			return
		}
		id := v.GetId()
		if id <= 0 {
			return
		}
		if _, ok := ids[id]; ok {
			return
		}
		if _, ok := ssa.ToConstInst(v); ok {
			if isSeed {
				ids[id] = struct{}{}
			}
			return
		}
		ids[id] = struct{}{}
		if pid := paramObjectID(v); pid > 0 {
			ids[pid] = struct{}{}
		}
		if isPointerish(v) {
			if _, isCall := ssa.ToCall(v); !isCall {
				for _, u := range v.GetUsers() {
					if u == nil || u.GetId() <= 0 {
						continue
					}
					ids[u.GetId()] = struct{}{}
				}
			}
		}
		if c, ok := ssa.ToCall(v); ok && c != nil {
			for _, aid := range c.Args {
				if aid <= 0 {
					continue
				}
				av, ok := c.GetValueById(aid)
				if !ok || av == nil {
					continue
				}
				if !isPointerish(av) {
					continue
				}
				walk(av, depth+1, false)
			}
			return
		}
		if v.HasValues() {
			for _, op := range v.GetValues() {
				walk(op, depth+1, false)
			}
		}
		if v.IsMember() {
			if obj := v.GetObject(); obj != nil {
				walk(obj, depth+1, false)
			}
		}
	}
	for _, s := range seeds {
		walk(s, 0, true)
	}
	return ids
}

func findingRelatedToSeeds(f *Finding, seedIDs map[int64]struct{}, reg *registry) bool {
	if f == nil || len(seedIDs) == 0 {
		return false
	}
	if _, ok := seedIDs[f.FreedObj]; ok {
		return true
	}
	if f.FreeCall != nil {
		if _, ok := seedIDs[f.FreeCall.GetId()]; ok {
			return true
		}
	}
	use := f.Use
	if use == nil {
		return false
	}
	if _, ok := seedIDs[use.GetId()]; ok {
		return true
	}
	inSeeds := func(v ssa.Value) bool {
		if v == nil || v.GetId() <= 0 {
			return false
		}
		_, ok := seedIDs[v.GetId()]
		return ok
	}
	if inSeeds(starDerefPointer(use)) {
		return true
	}
	if inSeeds(registeredDerefPointer(use, reg)) {
		return true
	}
	if obj := use.GetObject(); obj != nil {
		seen := map[int64]struct{}{}
		for depth := 0; obj != nil && depth < 8; obj, depth = obj.GetObject(), depth+1 {
			oid := obj.GetId()
			if oid <= 0 {
				break
			}
			if _, ok := seen[oid]; ok {
				break
			}
			seen[oid] = struct{}{}
			if _, ok := seedIDs[oid]; ok {
				return true
			}
			if !obj.IsMember() {
				break
			}
		}
	}
	if use.HasValues() {
		for _, op := range use.GetValues() {
			if op == nil {
				continue
			}
			if _, ok := ssa.ToConstInst(op); ok {
				continue
			}
			if _, ok := seedIDs[op.GetId()]; ok {
				return true
			}
		}
	}
	return false
}

func isNullish(v ssa.Value) bool {
	if utils.IsNil(v) {
		return true
	}
	c, ok := ssa.ToConstInst(v)
	if ok && c != nil {
		if c.Const != nil && c.Const.GetRawValue() == nil {
			return true
		}
		s := strings.ToLower(strings.TrimSpace(c.String()))
		if s == "0" || s == "null" || s == "nil" || s == "<nil>" || s == "nullptr" {
			return true
		}
	}
	// Extern / macro identifiers (e.g. unresolved NULL).
	name := strings.ToLower(strings.TrimSpace(v.GetName()))
	if name == "null" || name == "nullptr" || name == "nil" {
		return true
	}
	s := strings.ToLower(strings.TrimSpace(v.String()))
	return s == "null" || s == "nullptr" || s == "nil" || s == "<nil>"
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

func discoverAllocs(fn *ssa.Function, reg *registry) map[int64]struct{} {
	allocs := map[int64]struct{}{}
	mark := func(v ssa.Value) {
		if v == nil || v.GetId() <= 0 {
			return
		}
		id := v.GetId()
		if isHeapAllocValue(v) {
			allocs[id] = struct{}{}
			return
		}
		// Only consult registry for values that appear in this function —
		// do not copy the whole-program alloc set into every function.
		if reg != nil {
			reg.mu.RLock()
			_, ok := reg.alloc[id]
			reg.mu.RUnlock()
			if ok {
				allocs[id] = struct{}{}
			}
		}
	}
	if fn == nil {
		return allocs
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

// paramObjectID maps a value to its formal-parameter abstract object id when
// the value is the parameter itself or a member rooted at a Parameter
// (e.g. name "#15.@value" / string "parameter[0].@value").

// isUndefinedValue reports SSA Undefined placeholders (e.g. pre-created #phi.x).
func isUndefinedValue(v ssa.Value) bool {
	if v == nil {
		return false
	}
	_, ok := ssa.ToUndefined(v)
	return ok
}

// isPointerMetaMember reports EmitConstPointer / heap-pointer bookkeeping
// members (@pointer identity strings, $pval_*) that are not real C uses.
func isPointerMetaMember(v ssa.Value) bool {
	if v == nil || !v.IsMember() {
		return false
	}
	name := v.GetName()
	if name == "" {
		name = v.String()
	}
	if strings.HasPrefix(name, "$pval_") || strings.HasPrefix(name, "$heap_") {
		return true
	}
	if strings.Contains(name, "@pointer") {
		return true
	}
	s := strings.TrimSpace(v.String())
	return s == "@pointer" || s == "@value"
}

// starDerefPointer returns the pointer operand of a c2ssa bare-* artifact.
// Local null/*p often lowers as BinOp: <nil> mul <nullish> (missing left).
// Do NOT treat ordinary integer `x * 0` as a deref.
func starDerefPointer(v ssa.Value) ssa.Value {
	if v == nil {
		return nil
	}
	bin, ok := ssa.ToBinOp(v)
	if !ok || bin == nil || bin.Op != ssa.OpMul {
		return nil
	}
	x, xok := bin.GetValueById(bin.X)
	y, yok := bin.GetValueById(bin.Y)
	xMissing := !xok || x == nil
	yMissing := !yok || y == nil
	if xMissing && !yMissing && isNullish(y) {
		return y
	}
	if yMissing && !xMissing && isNullish(x) {
		return x
	}
	if !xMissing {
		if t := x.GetType(); t != nil && t.GetTypeKind() == ssa.PointerKind {
			return x
		}
	}
	if !yMissing {
		if t := y.GetType(); t != nil && t.GetTypeKind() == ssa.PointerKind {
			return y
		}
	}
	return nil
}

func registeredDerefPointer(v ssa.Value, reg *registry) ssa.Value {
	if v == nil || reg == nil {
		return nil
	}
	ptrID, ok := reg.derefPtr(v.GetId())
	if !ok || ptrID <= 0 {
		return nil
	}
	if ptr, ok := v.GetValueById(ptrID); ok && ptr != nil {
		return ptr
	}
	if fn := v.GetFunc(); fn != nil {
		if ptr, ok := fn.GetValueById(ptrID); ok && ptr != nil {
			return ptr
		}
	}
	return nil
}

func paramObjectID(v ssa.Value) int64 {
	if v == nil {
		return 0
	}
	if p, ok := ssa.ToParameter(v); ok && p != nil && !p.IsFreeValue {
		return p.GetId()
	}
	// Walk member parents: parameter[i].@value is a member of Parameter p.
	seen := map[int64]struct{}{}
	cur := v
	for depth := 0; cur != nil && cur.IsMember() && depth < 4; depth++ {
		id := cur.GetId()
		if _, ok := seen[id]; ok {
			break
		}
		seen[id] = struct{}{}
		obj := cur.GetObject()
		if obj == nil {
			break
		}
		if p, ok := ssa.ToParameter(obj); ok && p != nil && !p.IsFreeValue {
			return p.GetId()
		}
		cur = obj
	}
	name := v.GetName()
	if name == "" || !strings.HasPrefix(name, "parameter[") {
		name = v.String()
	}
	if strings.HasPrefix(name, "parameter[") {
		end := strings.IndexByte(name, ']')
		if end > len("parameter[") {
			idxStr := name[len("parameter[") : end]
			idx := 0
			okNum := true
			for i := 0; i < len(idxStr); i++ {
				c := idxStr[i]
				if c < '0' || c > '9' {
					okNum = false
					break
				}
				idx = idx*10 + int(c-'0')
			}
			if okNum {
				if fn := v.GetFunc(); fn != nil && idx >= 0 && idx < len(fn.Params) {
					return fn.Params[idx]
				}
			}
		}
	}
	return 0
}

// collectParamAliases maps SSA value ids that are must-aliases of a formal
// parameter (the formal itself, pure copies, casts, and non-conflicting phis)
// to that parameter's index. Used by free-param summary for free(q) where q=p.
func collectParamAliases(fn *ssa.Function) map[int64]int {
	out := make(map[int64]int)
	if fn == nil {
		return out
	}
	for i, pid := range fn.Params {
		if pid > 0 {
			out[pid] = i
		}
	}
	for iter := 0; iter < 32; iter++ {
		changed := false
		for _, bid := range fn.Blocks {
			b, ok := fn.GetBasicBlockByID(bid)
			if !ok || b == nil {
				continue
			}
			scan := func(iid int64) {
				inst, ok := b.GetInstructionById(iid)
				if !ok || inst == nil {
					return
				}
				inst = resolveInstruction(inst)
				v, ok := inst.(ssa.Value)
				if !ok || v == nil || v.GetId() <= 0 {
					return
				}
				id := v.GetId()
				if _, ok := out[id]; ok {
					return
				}
				if _, ok := ssa.ToCall(v); ok {
					return
				}
				if isNullish(v) || isHeapAllocValue(v) {
					return
				}
				ops := v.GetValues()
				if len(ops) == 0 {
					return
				}
				seenIdx := -1
				conflict := false
				knownOps := 0
				for _, op := range ops {
					if op == nil {
						continue
					}
					idx, ok := out[op.GetId()]
					if !ok {
						continue
					}
					knownOps++
					if seenIdx < 0 {
						seenIdx = idx
					} else if seenIdx != idx {
						conflict = true
					}
				}
				if conflict || seenIdx < 0 {
					return
				}
				_, isPhi := ssa.ToPhi(v)
				_, isCast := ssa.ToTypeCast(v)
				// Pure copy / cast / phi of a single formal alias only.
				if isPhi || isCast || len(ops) == 1 || knownOps == len(ops) && len(ops) <= 2 && !v.IsMember() {
					if isPhi || isCast || len(ops) == 1 {
						out[id] = seenIdx
						changed = true
					}
				}
			}
			for _, pid := range b.Phis {
				scan(pid)
			}
			for _, iid := range b.Insts {
				scan(iid)
			}
		}
		if !changed {
			break
		}
	}
	return out
}

// classifyFreeArg reports whether a free() argument is a formal (value)
// or a dereference of a formal (*p / origin-of-p).
func classifyFreeArg(av ssa.Value, aliases map[int64]int) (idx int, throughStar bool, ok bool) {
	if av == nil || aliases == nil {
		return 0, false, false
	}
	if i, hit := aliases[av.GetId()]; hit {
		return i, false, true
	}
	if p, pok := ssa.ToParameter(av); pok && p != nil && !p.IsFreeValue {
		return p.FormalParameterIndex, false, true
	}
	if av.IsMember() {
		if obj := av.GetObject(); obj != nil {
			if i, hit := aliases[obj.GetId()]; hit {
				return i, true, true
			}
			if p, pok := ssa.ToParameter(obj); pok && p != nil && !p.IsFreeValue {
				return p.FormalParameterIndex, true, true
			}
		}
	}
	if pid := paramObjectID(av); pid > 0 && pid != av.GetId() {
		if i, hit := aliases[pid]; hit {
			return i, true, true
		}
	}
	return 0, false, false
}

// pointerPointeeIDs returns @value / origin objects of a pointer (e.g. &p → p).
func pointerPointeeIDs(v ssa.Value) []int64 {
	if v == nil {
		return nil
	}
	seen := map[int64]struct{}{}
	var ids []int64
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
	if m, ok := v.GetStringMember("@value"); ok {
		add(m)
	}
	for _, m := range v.GetMembersByKeyString("@value") {
		add(m)
	}
	if v.IsMember() {
		if key := v.GetKey(); key != nil && strings.Contains(key.String(), "@value") {
			add(v)
		}
	}
	return ids
}

// applyNullGuardOnEdge refines nullness when control flows from an If
// predecessor into its true/false successor (if (p) / if (!p) / comparisons).
func applyNullGuardOnEdge(pred, succ *ssa.BasicBlock, st *npdState) {
	if pred == nil || succ == nil || st == nil {
		return
	}
	succID := succ.GetId()
	for _, iid := range pred.Insts {
		inst, ok := pred.GetInstructionById(iid)
		if !ok || inst == nil {
			continue
		}
		inst = resolveInstruction(inst)
		ifInst, ok := ssa.ToIfInstruction(inst)
		if !ok || ifInst == nil {
			continue
		}
		enteringTrue := ifInst.True == succID
		enteringFalse := ifInst.False == succID
		if !enteringTrue && !enteringFalse {
			continue
		}
		cond, ok := ifInst.GetValueById(ifInst.Cond)
		if !ok || cond == nil {
			continue
		}
		target, wantNull := resolveNullCheckTarget(cond)
		if target == nil || target.GetId() <= 0 {
			continue
		}
		// if (p) true → NonNull; if (p) false → IsNull
		// if (!p) true → IsNull; if (!p) false → NonNull
		var ns NullState
		if enteringTrue {
			if wantNull {
				ns = NullIsNull
			} else {
				ns = NullNonNull
			}
		} else {
			if wantNull {
				ns = NullNonNull
			} else {
				ns = NullIsNull
			}
		}
		st.set(target.GetId(), ns)
		if pid := paramObjectID(target); pid > 0 {
			st.set(pid, ns)
		}
	}
}

// resolveNullCheckTarget returns the pointer under test and whether the
// condition means "is null" (true) or "is non-null" (false) when the
// condition itself is true.
func resolveNullCheckTarget(cond ssa.Value) (ssa.Value, bool) {
	if cond == nil {
		return nil, false
	}
	if u, ok := ssa.ToUnOp(cond); ok && u != nil && u.Op == ssa.OpNot {
		x, ok := cond.GetValueById(u.X)
		if !ok || x == nil {
			return nil, false
		}
		inner, wantNull := resolveNullCheckTarget(x)
		if inner == nil {
			return nil, false
		}
		return inner, !wantNull
	}
	if bin, ok := ssa.ToBinOp(cond); ok && bin != nil {
		x, _ := cond.GetValueById(bin.X)
		y, _ := cond.GetValueById(bin.Y)
		switch bin.Op {
		case ssa.OpEq:
			if isNullish(x) && y != nil {
				return y, true
			}
			if isNullish(y) && x != nil {
				return x, true
			}
		case ssa.OpNotEq:
			if isNullish(x) && y != nil {
				return y, false
			}
			if isNullish(y) && x != nil {
				return x, false
			}
		}
	}
	// Bare `if (p)`: condition true ⇒ non-null.
	return cond, false
}
