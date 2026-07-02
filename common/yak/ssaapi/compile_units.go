package ssaapi

import (
	"sort"

	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// CompileUnit and UnitRef are owned by the ssa package so language Builders
// (in common/yak/<lang>2ssa) can produce them without importing ssaapi.
type CompileUnit = ssa.CompileUnit
type UnitRef = ssa.UnitRef

// UnitPlan is the engine-side result of partitioning + dependency extraction:
// the unit map, the dependency edges, and the units grouped into SCCs in
// topological order.
type UnitPlan struct {
	Units map[string]*CompileUnit
	Edges []UnitRef
	Order [][]*CompileUnit
}

// buildCompileUnitPlan delegates partitioning and dependency extraction to the
// language Builder, then topologically sorts the resulting units (collapsing
// cycles into SCCs). The engine fills CompileUnit.Language from the project
// language so language Builders need not set it themselves.
func buildCompileUnitPlan(builder ssa.Builder, language ssaconfig.Language, fs filesys_interface.FileSystem, files []string) *UnitPlan {
	files = append([]string(nil), files...)
	unitSlice := builder.PartitionCompileUnits(fs, files)
	units := make(map[string]*CompileUnit, len(unitSlice))
	for _, u := range unitSlice {
		if u == nil {
			continue
		}
		u.Language = language
		units[u.Key] = u
	}
	edges := builder.CompileUnitDependencies(fs, unitSlice)
	return topoCompileUnits(units, edges)
}

func topoCompileUnits(units map[string]*CompileUnit, edges []UnitRef) *UnitPlan {
	sccs := stronglyConnectedUnits(units, edges)
	sccIndex := make(map[string]int)
	for idx, scc := range sccs {
		for _, unit := range scc {
			sccIndex[unit.Key] = idx
		}
	}
	out := make(map[int]map[int]struct{})
	indegree := make(map[int]int)
	for i := range sccs {
		indegree[i] = 0
	}
	for _, edge := range edges {
		from, fromOK := sccIndex[edge.From]
		to, toOK := sccIndex[edge.To]
		if !fromOK || !toOK || from == to {
			continue
		}
		if out[to] == nil {
			out[to] = make(map[int]struct{})
		}
		if _, exists := out[to][from]; !exists {
			out[to][from] = struct{}{}
			indegree[from]++
		}
	}
	queue := make([]int, 0)
	for idx, degree := range indegree {
		if degree == 0 {
			queue = append(queue, idx)
		}
	}
	sort.Ints(queue)
	var order [][]*CompileUnit
	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]
		order = append(order, sccs[idx])
		next := make([]int, 0, len(out[idx]))
		for dep := range out[idx] {
			indegree[dep]--
			if indegree[dep] == 0 {
				next = append(next, dep)
			}
		}
		sort.Ints(next)
		queue = append(queue, next...)
	}
	if len(order) != len(sccs) {
		order = sccs
	}
	return &UnitPlan{Units: units, Edges: edges, Order: order}
}

func stronglyConnectedUnits(units map[string]*CompileUnit, edges []UnitRef) [][]*CompileUnit {
	keys := make([]string, 0, len(units))
	for key := range units {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	graph := make(map[string][]string)
	for _, edge := range edges {
		if units[edge.From] == nil || units[edge.To] == nil {
			continue
		}
		graph[edge.From] = append(graph[edge.From], edge.To)
	}
	for key := range graph {
		sort.Strings(graph[key])
	}
	index := 0
	stack := make([]string, 0)
	onStack := make(map[string]bool)
	indices := make(map[string]int)
	lowlink := make(map[string]int)
	var sccs [][]*CompileUnit
	var visit func(string)
	visit = func(v string) {
		indices[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range graph[v] {
			if _, seen := indices[w]; !seen {
				visit(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] && indices[w] < lowlink[v] {
				lowlink[v] = indices[w]
			}
		}
		if lowlink[v] != indices[v] {
			return
		}
		var scc []*CompileUnit
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			scc = append(scc, units[w])
			if w == v {
				break
			}
		}
		sort.Slice(scc, func(i, j int) bool { return scc[i].Key < scc[j].Key })
		sccs = append(sccs, scc)
	}
	for _, key := range keys {
		if _, seen := indices[key]; !seen {
			visit(key)
		}
	}
	sort.SliceStable(sccs, func(i, j int) bool { return sccs[i][0].Key < sccs[j][0].Key })
	return sccs
}
