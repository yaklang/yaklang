package lifetime

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Finding is a lifetime / nullness violation site.
// Double-free is a UAF subtype. NPD is independent (KindNPD).
// For KindNPD, FreedObj holds the pointer value id that was Null/MaybeNull.
type Finding struct {
	Use      ssa.Value // violating use (deref / 2nd free / …)
	FreedObj int64     // heap/abstract object id; for NPD: pointer value id
	FreeCall ssa.Value // optional prior free (UAF/double-free only)
	Kind     string    // KindUAF | KindDoubleFree | KindNPD
}

// Finding kinds returned by FindUAFUses / FindNPDUses.
const (
	KindUAF        = "uaf"
	KindDoubleFree = "double-free" // UAF subtype: free after free
	KindNPD        = "npd"         // null pointer dereference (independent of UAF)
)

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

// paramObjectID maps a value to its formal-parameter abstract object id when
// the value is the parameter itself or a member rooted at a Parameter
// (e.g. name "#15.@value" / string "parameter[0].@value").
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
