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

var (
	progRegs sync.Map // *ssa.Program -> *registry
)

func getReg(prog *ssa.Program) *registry {
	if prog == nil {
		return nil
	}
	if v, ok := progRegs.Load(prog); ok {
		return v.(*registry)
	}
	r := &registry{
		alloc:  make(map[int64]struct{}),
		kills:  make(map[int64][]int64),
		derefs: make(map[int64]int64),
	}
	actual, _ := progRegs.LoadOrStore(prog, r)
	return actual.(*registry)
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

func (r *registry) killArgs(callID int64) []int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]int64(nil), r.kills[callID]...)
}
