package ssaapi

func (v *Value) GetDataflowPath(end ...*Value) []Values {
	var paths []Values

	endSet := make(map[int64]struct{})
	hasEnd := len(end) > 0
	if hasEnd {
		for _, e := range end {
			endSet[e.GetId()] = struct{}{}
		}
	}

	var dfs func(current *Value, path Values, visited map[int64]bool)
	dfs = func(current *Value, path Values, visited map[int64]bool) {
		if visited[current.GetId()] {
			return
		}
		visited[current.GetId()] = true
		defer func() { visited[current.GetId()] = false }()

		newPath := append(path, current)

		_, isEnd := endSet[current.GetId()]
		succs := current.GetDataFlow()

		if hasEnd {
			if isEnd {
				paths = append(paths, newPath)
				return
			}
		} else {
			if len(succs) == 0 {
				paths = append(paths, newPath)
			}
		}

		for _, succ := range succs {
			dfs(succ, newPath, visited)
		}
	}

	dfs(v, Values{}, make(map[int64]bool))

	return paths
}

func (vs Values) GetDataflowPath(end ...*Value) []Values {
	path := make([]Values, 0)
	for _, v := range vs {
		path = append(path, v.GetDataflowPath(end...)...)
	}
	return path
}
