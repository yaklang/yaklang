package lifetime

import (
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

// programState holds per-Program analysis caches: registry, function index,
// callee summaries, and memoized full-program findings.
type programState struct {
	reg *registry

	indexOnce sync.Once
	byID      map[int64]*ssa.Function
	byName    map[string][]*ssa.Function // exact name and basename keys

	summaryOnce sync.Once
	summaries   *calleeSummaries

	uafOnce     sync.Once
	uafFindings []*Finding

	npdOnce     sync.Once
	npdFindings []*Finding
}

var progStates sync.Map // *ssa.Program -> *programState

func getState(prog *ssa.Program) *programState {
	if prog == nil {
		return nil
	}
	if v, ok := progStates.Load(prog); ok {
		return v.(*programState)
	}
	st := &programState{
		reg: &registry{
			alloc:  make(map[int64]struct{}),
			kills:  make(map[int64][]int64),
			derefs: make(map[int64]int64),
		},
	}
	actual, _ := progStates.LoadOrStore(prog, st)
	return actual.(*programState)
}

// getReg returns the heap/kill/deref registry for prog (via programState).
func getReg(prog *ssa.Program) *registry {
	st := getState(prog)
	if st == nil {
		return nil
	}
	return st.reg
}

func (st *programState) ensureIndex(prog *ssa.Program) {
	if st == nil || prog == nil {
		return
	}
	st.indexOnce.Do(func() {
		st.byID = make(map[int64]*ssa.Function)
		st.byName = make(map[string][]*ssa.Function)
		prog.EachFunction(func(fn *ssa.Function) {
			if fn == nil || fn.GetId() <= 0 {
				return
			}
			st.byID[fn.GetId()] = fn
			name := fn.GetName()
			if name == "" {
				return
			}
			st.byName[name] = append(st.byName[name], fn)
			base := name
			if i := strings.LastIndex(base, "."); i >= 0 {
				base = base[i+1:]
			}
			base = strings.TrimPrefix(base, "Function-")
			if base != "" && base != name {
				st.byName[base] = append(st.byName[base], fn)
			}
		})
	})
}

func (st *programState) funcByID(id int64) *ssa.Function {
	if st == nil || id <= 0 {
		return nil
	}
	return st.byID[id]
}

func (st *programState) funcsByName(name string) []*ssa.Function {
	if st == nil || name == "" {
		return nil
	}
	return st.byName[name]
}

func (st *programState) ensureSummaries(prog *ssa.Program) *calleeSummaries {
	if st == nil || prog == nil {
		return &calleeSummaries{
			freeParams:      make(map[int64]map[int]struct{}),
			freeDerefParams: make(map[int64]map[int]struct{}),
			derefParams:     make(map[int64]map[int]struct{}),
		}
	}
	st.summaryOnce.Do(func() {
		st.ensureIndex(prog)
		st.summaries = buildCalleeSummaries(prog, st.reg)
	})
	return st.summaries
}
