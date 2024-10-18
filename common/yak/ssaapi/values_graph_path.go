package ssaapi

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
)

func (v *Value) GetGraphDependOnNeighbors() Values {
	var vals Values
	if len(v.Predecessors) > 0 {
		for _, p := range v.Predecessors {
			vals = append(vals, p.Node)
		}
	}
	vals = append(vals, v.DependOn...)
	return vals
}

func (v *Value) GetGraphEffectOnNeighbors() Values {
	var vals Values
	if len(v.Predecessors) > 0 {
		for _, s := range v.Predecessors {
			vals = append(vals, s.Node)
		}
	}
	vals = append(vals, v.EffectOn...)
	return vals
}

type dfsHandler func(start *Value, isEnd func(*Value) bool, visited map[*Value]struct{}, path Values, callback func(Values))

func (v *Value) GetDependOnPaths(end Values) []Values {
	vs := Values{v}
	return vs.getPathsWithHandler(end, vs.dfsDependOn)
}

func (v *Value) GetEffectOnPaths(end Values) []Values {
	vs := Values{v}
	return vs.getPathsWithHandler(end, vs.dfsEffectOn)
}

func (v *Value) GetPaths(end Values) []Values {
	vs := Values{v}
	return append(vs.GetDependOnPaths(end), vs.GetEffectOnPaths(end)...)
}

func (vs Values) GetDependOnPaths(end Values) []Values {
	return vs.getPathsWithHandler(end, vs.dfsDependOn)
}

func (vs Values) GetEffectOnPaths(end Values) []Values {
	return vs.getPathsWithHandler(end, vs.dfsEffectOn)
}

func (vs Values) GetPaths(end Values) []Values {
	return append(vs.GetDependOnPaths(end), vs.GetEffectOnPaths(end)...)
}

func (vs Values) getPathsWithHandler(end Values, handle dfsHandler) []Values {
	var paths []Values

	target := make(map[string]struct{})
	idTarget := make(map[int64]struct{})

	for _, endValue := range end {
		if endValue.GetId() > 0 {
			idTarget[endValue.GetId()] = struct{}{}
			continue
		} else if endValue.IsConstInst() {
			lit := endValue.GetConstValue()
			if ret := fmt.Sprint(lit); ret != "" {
				target[ret] = struct{}{}
				continue
			}
		} else {
			hash := utils.CalcSha256(endValue.String())
			target[hash] = struct{}{}
		}
	}

	// get all paths from start to end
	for _, start := range vs {
		handle(start, func(value *Value) bool {
			_, ok := idTarget[value.GetId()]
			if ok {
				return true
			}

			if value.IsConstInst() {
				ret := fmt.Sprint(value.GetConstValue())
				if ret != "" {
					_, ok := target[ret]
					if ok {
						return true
					}
				}
			}

			hash := utils.CalcSha256(value.String())
			_, ok = target[hash]
			if ok {
				return true
			}
			return false
		}, make(map[*Value]struct{}), Values{}, func(path Values) {
			paths = append(paths, path)
		})
	}

	return paths
}

func (vs Values) dfsDependOn(start *Value, isEnd func(*Value) bool, visited map[*Value]struct{}, path Values, callback func(Values)) {
	visited[start] = struct{}{}
	path = append(path, start)
	if isEnd(start) {
		callback(path)
	} else {
		for _, neighbor := range start.GetGraphDependOnNeighbors() {
			if _, ok := visited[neighbor]; !ok {
				vs.dfsDependOn(neighbor, isEnd, visited, path, callback)
			}
		}
	}
	delete(visited, start)
}

func (vs Values) dfsEffectOn(start *Value, isEnd func(*Value) bool, visited map[*Value]struct{}, path Values, callback func(Values)) {
	visited[start] = struct{}{}
	path = append(path, start)
	if isEnd(start) {
		callback(path)
	} else {
		for _, neighbor := range start.GetGraphEffectOnNeighbors() {
			if _, ok := visited[neighbor]; !ok {
				vs.dfsEffectOn(neighbor, isEnd, visited, path, callback)
			}
		}
	}
	delete(visited, start)
}
