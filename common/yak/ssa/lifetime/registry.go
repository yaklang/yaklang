package lifetime

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// ObjectState is the lifetime state of a heap object on a CFG path.
type ObjectState int

const (
	StateAlive ObjectState = iota
	StateFreed
)

// registry tracks heap allocation sites and optional free kills for a program.
type registry struct {
	mu     sync.RWMutex
	alloc  map[int64]struct{} // value id -> heap object (obj id == value id)
	kills  map[int64][]int64  // free-call id -> argument value ids
	derefs map[int64]int64    // explicit *p load site id -> pointer object id
}

// RegisterAlloc marks v as a heap allocation site (object id == v.GetId()).
// Also stamps a durable VerboseName so analysis works after DB reload.
func RegisterAlloc(v ssa.Value) {
	if v == nil || v.GetId() <= 0 {
		return
	}
	prog := v.GetProgram()
	r := getReg(prog)
	if r == nil {
		return
	}
	r.mu.Lock()
	r.alloc[v.GetId()] = struct{}{}
	r.mu.Unlock()
	// Survive program reload from database (in-memory registry is lost).
	if vn := v.GetVerboseName(); vn == "" || !strings.HasPrefix(vn, HeapAllocVerbosePrefix) {
		v.SetVerboseName(HeapAllocVerbosePrefix + fmt.Sprintf("%d", v.GetId()))
	}
}

// HeapAllocVerbosePrefix marks pointer values created by heap alloc wrappers.
const HeapAllocVerbosePrefix = "__HeapAlloc__"

// RegisterKill records that call frees the given pointer argument values.
func RegisterKill(call ssa.Value, args ...ssa.Value) {
	if call == nil || call.GetId() <= 0 {
		return
	}
	prog := call.GetProgram()
	r := getReg(prog)
	if r == nil {
		return
	}
	ids := make([]int64, 0, len(args))
	for _, a := range args {
		if a != nil && a.GetId() > 0 {
			ids = append(ids, a.GetId())
		}
	}
	if len(ids) == 0 {
		return
	}
	r.mu.Lock()
	r.kills[call.GetId()] = ids
	r.mu.Unlock()
}

// RegisterDeref records an explicit pointer dereference load site (*p as rvalue).
// Needed when the @value payload ConstInst is reused as both construction and load,
// so member-walk alone cannot see a distinct use instruction.
func RegisterDeref(site, ptr ssa.Value) {
	if site == nil || ptr == nil || site.GetId() <= 0 || ptr.GetId() <= 0 {
		return
	}
	prog := site.GetProgram()
	if prog == nil {
		prog = ptr.GetProgram()
	}
	r := getReg(prog)
	if r == nil {
		return
	}
	r.mu.Lock()
	r.derefs[site.GetId()] = ptr.GetId()
	r.mu.Unlock()
}

func (r *registry) derefPtr(siteID int64) (int64, bool) {
	if r == nil || siteID <= 0 {
		return 0, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.derefs[siteID]
	return id, ok
}

// IsAlloc reports whether value id is a registered heap allocation.
func IsAlloc(prog *ssa.Program, id int64) bool {
	r := getReg(prog)
	if r == nil {
		return false
	}
	r.mu.RLock()
	_, ok := r.alloc[id]
	r.mu.RUnlock()
	return ok
}

func (r *registry) snapshotAlloc() map[int64]struct{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[int64]struct{}, len(r.alloc))
	for k := range r.alloc {
		out[k] = struct{}{}
	}
	return out
}

func (r *registry) snapshotKills() map[int64][]int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[int64][]int64, len(r.kills))
	for k, v := range r.kills {
		out[k] = append([]int64(nil), v...)
	}
	return out
}

func (r *registry) snapshotDerefs() map[int64]int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[int64]int64, len(r.derefs))
	for k, v := range r.derefs {
		out[k] = v
	}
	return out
}

func (r *registry) killArgs(callID int64) []int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]int64(nil), r.kills[callID]...)
}

func resolveProgValue(prog *ssa.Program, id int64) ssa.Value {
	if prog == nil || id <= 0 {
		return nil
	}
	inst, ok := prog.GetInstructionById(id)
	if !ok || inst == nil {
		return nil
	}
	inst = resolveInstruction(inst)
	v, _ := inst.(ssa.Value)
	return v
}

// ListHeapAllocs returns registered / stamped heap allocation values in prog.
func ListHeapAllocs(prog *ssa.Program) []ssa.Value {
	if prog == nil {
		return nil
	}
	seen := make(map[int64]struct{})
	var out []ssa.Value
	add := func(v ssa.Value) {
		if v == nil || v.GetId() <= 0 {
			return
		}
		id := v.GetId()
		if _, ok := seen[id]; ok {
			return
		}
		if !isHeapAllocValue(v) && !IsAlloc(prog, id) {
			return
		}
		seen[id] = struct{}{}
		out = append(out, v)
	}
	r := getReg(prog)
	if r != nil {
		for id := range r.snapshotAlloc() {
			add(resolveProgValue(prog, id))
		}
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
			for _, iid := range append(append([]int64{}, b.Phis...), b.Insts...) {
				inst, ok := b.GetInstructionById(iid)
				if !ok || inst == nil {
					continue
				}
				if v, ok := resolveInstruction(inst).(ssa.Value); ok {
					add(v)
				}
			}
		}
	})
	return out
}

// ListFreeCalls returns free / RegisterKill call sites in prog.
func ListFreeCalls(prog *ssa.Program) []ssa.Value {
	if prog == nil {
		return nil
	}
	seen := make(map[int64]struct{})
	var out []ssa.Value
	add := func(v ssa.Value) {
		if v == nil || v.GetId() <= 0 {
			return
		}
		id := v.GetId()
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, v)
	}
	r := getReg(prog)
	if r != nil {
		for id := range r.snapshotKills() {
			add(resolveProgValue(prog, id))
		}
	}
	// Also pick up free() calls that may lack RegisterKill (e.g. DB reload).
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
				call, ok := ssa.ToCall(resolveInstruction(inst))
				if !ok || call == nil || !isFreeCall(call, r) {
					continue
				}
				add(call)
			}
		}
	})
	return out
}

// ListDerefSites returns explicit *p load sites registered via RegisterDeref.
// After DB reload the in-memory registry is empty; sites may be missing.
func ListDerefSites(prog *ssa.Program) []ssa.Value {
	if prog == nil {
		return nil
	}
	r := getReg(prog)
	if r == nil {
		return nil
	}
	var out []ssa.Value
	seen := make(map[int64]struct{})
	for siteID := range r.snapshotDerefs() {
		v := resolveProgValue(prog, siteID)
		if v == nil || v.GetId() <= 0 {
			continue
		}
		if _, ok := seen[v.GetId()]; ok {
			continue
		}
		seen[v.GetId()] = struct{}{}
		out = append(out, v)
	}
	return out
}

// ListHeapAllocsRelated filters ListHeapAllocs by pointer/object seed relatedness.
func ListHeapAllocsRelated(prog *ssa.Program, seeds []ssa.Value) []ssa.Value {
	return filterValuesBySeeds(ListHeapAllocs(prog), seeds)
}

// ListFreeCallsRelated filters ListFreeCalls by seed relatedness (call or killed args).
func ListFreeCallsRelated(prog *ssa.Program, seeds []ssa.Value) []ssa.Value {
	if len(seeds) == 0 {
		return nil
	}
	seedIDs := expandLifetimeSeedIDs(seeds)
	if len(seedIDs) == 0 {
		return nil
	}
	r := getReg(prog)
	var out []ssa.Value
	for _, c := range ListFreeCalls(prog) {
		if c == nil {
			continue
		}
		if _, ok := seedIDs[c.GetId()]; ok {
			out = append(out, c)
			continue
		}
		if r != nil {
			for _, aid := range r.killArgs(c.GetId()) {
				if _, ok := seedIDs[aid]; ok {
					out = append(out, c)
					break
				}
			}
		}
		if call, ok := ssa.ToCall(c); ok && call != nil {
			for _, aid := range call.Args {
				if _, ok := seedIDs[aid]; ok {
					out = append(out, c)
					break
				}
			}
		}
	}
	return out
}

// ListDerefSitesRelated filters deref sites whose pointer operand relates to seeds.
func ListDerefSitesRelated(prog *ssa.Program, seeds []ssa.Value) []ssa.Value {
	if len(seeds) == 0 {
		return nil
	}
	seedIDs := expandLifetimeSeedIDs(seeds)
	if len(seedIDs) == 0 {
		return nil
	}
	r := getReg(prog)
	var out []ssa.Value
	for _, site := range ListDerefSites(prog) {
		if site == nil {
			continue
		}
		if _, ok := seedIDs[site.GetId()]; ok {
			out = append(out, site)
			continue
		}
		if r != nil {
			if ptrID, ok := r.derefPtr(site.GetId()); ok {
				if _, hit := seedIDs[ptrID]; hit {
					out = append(out, site)
					continue
				}
			}
		}
		if ptr := registeredDerefPointer(site, r); ptr != nil {
			if _, ok := seedIDs[ptr.GetId()]; ok {
				out = append(out, site)
			}
		}
	}
	return out
}

func filterValuesBySeeds(vals []ssa.Value, seeds []ssa.Value) []ssa.Value {
	if len(seeds) == 0 {
		return nil
	}
	seedIDs := expandLifetimeSeedIDs(seeds)
	if len(seedIDs) == 0 {
		return nil
	}
	var out []ssa.Value
	for _, v := range vals {
		if v == nil {
			continue
		}
		if _, ok := seedIDs[v.GetId()]; ok {
			out = append(out, v)
			continue
		}
		if pid := paramObjectID(v); pid > 0 {
			if _, ok := seedIDs[pid]; ok {
				out = append(out, v)
			}
		}
	}
	return out
}
