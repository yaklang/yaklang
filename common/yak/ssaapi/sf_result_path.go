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

func (r *SyntaxFlowResult) GetDependOnPaths(start, end string) []Values {
	return r.getPathsWithHandler(start, end, r.dfsDependOn)
}

func (r *SyntaxFlowResult) GetEffectOnPaths(start, end string) []Values {
	return r.getPathsWithHandler(start, end, r.dfsEffectOn)
}

func (r *SyntaxFlowResult) GetPaths(start, end string) []Values {
	return append(r.GetDependOnPaths(start, end), r.GetEffectOnPaths(start, end)...)
}

func (r *SyntaxFlowResult) getPathsWithHandler(start, end string, handle dfsHandler) []Values {
	var paths []Values

	target := make(map[string]struct{})
	idTarget := make(map[int64]struct{})

	for _, endValue := range r.GetValues(end) {
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
	for _, start := range r.GetValues(start) {
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

func (s *SyntaxFlowResult) dfsDependOn(start *Value, isEnd func(*Value) bool, visited map[*Value]struct{}, path Values, callback func(Values)) {
	visited[start] = struct{}{}
	path = append(path, start)
	if isEnd(start) {
		callback(path)
	} else {
		for _, neighbor := range start.GetGraphDependOnNeighbors() {
			if _, ok := visited[neighbor]; !ok {
				s.dfsDependOn(neighbor, isEnd, visited, path, callback)
			}
		}
	}
	delete(visited, start)
}

func (s *SyntaxFlowResult) dfsEffectOn(start *Value, isEnd func(*Value) bool, visited map[*Value]struct{}, path Values, callback func(Values)) {
	visited[start] = struct{}{}
	path = append(path, start)
	if isEnd(start) {
		callback(path)
	} else {
		for _, neighbor := range start.GetGraphEffectOnNeighbors() {
			if _, ok := visited[neighbor]; !ok {
				s.dfsEffectOn(neighbor, isEnd, visited, path, callback)
			}
		}
	}
	delete(visited, start)
}
