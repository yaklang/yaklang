package ssaapi

import "context"

func (v *Value) GetDataflowPath(end ...*Value) []Values {
	return v.GetDataflowPathWithContext(context.Background(), end...)
}

// GetDataflowPathWithContext enumerates paths until ctx is cancelled. Callers
// that only need a matching path should use visitDataflowPaths to stop early.
func (v *Value) GetDataflowPathWithContext(ctx context.Context, end ...*Value) []Values {
	return v.GetDataflowPathWithEdgeFilterWithContext(ctx, nil, end...)
}

func (v *Value) GetEffectOnPath(end ...*Value) []Values {
	return v.GetEffectOnPathWithContext(context.Background(), end...)
}
func (v *Value) GetEffectOnPathWithContext(ctx context.Context, end ...*Value) []Values {
	return v.getPathWithDirectionWithContext(ctx, nil, (*Value).GetEffectOn, end...)
}

func (v *Value) GetDependOnPath(end ...*Value) []Values {
	return v.GetDependOnPathWithContext(context.Background(), end...)
}
func (v *Value) GetDependOnPathWithContext(ctx context.Context, end ...*Value) []Values {
	return v.getPathWithDirectionWithContext(ctx, nil, (*Value).GetDependOn, end...)
}

// GetEffectOnPathWithEdgeFilter drops edges for which edgeFilter returns false.
// A nil edgeFilter disables filtering.
func (v *Value) GetEffectOnPathWithEdgeFilter(edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.GetEffectOnPathWithEdgeFilterWithContext(context.Background(), edgeFilter, end...)
}
func (v *Value) GetEffectOnPathWithEdgeFilterWithContext(ctx context.Context, edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.getPathWithDirectionWithContext(ctx, edgeFilter, (*Value).GetEffectOn, end...)
}

// GetDependOnPathWithEdgeFilter drops edges for which edgeFilter returns false.
func (v *Value) GetDependOnPathWithEdgeFilter(edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.GetDependOnPathWithEdgeFilterWithContext(context.Background(), edgeFilter, end...)
}
func (v *Value) GetDependOnPathWithEdgeFilterWithContext(ctx context.Context, edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.getPathWithDirectionWithContext(ctx, edgeFilter, (*Value).GetDependOn, end...)
}

func (v *Value) GetDataflowPathWithEdgeFilter(edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.GetDataflowPathWithEdgeFilterWithContext(context.Background(), edgeFilter, end...)
}
func (v *Value) GetDataflowPathWithEdgeFilterWithContext(ctx context.Context, edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	var paths []Values
	v.visitDataflowPaths(ctx, edgeFilter, nil, func(path Values) bool {
		paths = append(paths, path)
		return true
	}, end...)
	return paths
}

// visitDataflowPaths streams the effect/depend Cartesian product in the same
// order within each path as GetDataflowPath: effect -> v -> depend. Neither
// direction nor the product is materialized in advance. Returning false from
// visit stops the entire traversal; bail charges the caller's shared budget.
func (v *Value) visitDataflowPaths(ctx context.Context, edgeFilter func(from, to *Value) bool, bail func() bool, visit func(Values) bool, end ...*Value) bool {
	emit := func(effect, depend Values) bool {
		if ctx.Err() != nil || (bail != nil && bail()) {
			return false
		}
		path := make(Values, 0, len(effect)+len(depend)+1)
		path = append(path, effect...)
		path = append(path, v)
		path = append(path, depend...)
		return visit(path)
	}
	hasEffect := false
	if !v.visitPathsWithDirection(ctx, edgeFilter, (*Value).GetEffectOn, bail, func(effect Values) bool {
		hasEffect = true
		hasDepend := false
		if !v.visitPathsWithDirection(ctx, edgeFilter, (*Value).GetDependOn, bail, func(depend Values) bool {
			hasDepend = true
			return emit(effect, depend)
		}, end...) {
			return false
		}
		return hasDepend || emit(effect, nil)
	}, end...) {
		return false
	}
	if !hasEffect {
		return v.visitPathsWithDirection(ctx, edgeFilter, (*Value).GetDependOn, bail, func(depend Values) bool {
			return emit(nil, depend)
		}, end...)
	}
	return true
}

func (v *Value) getPathWithDirectionWithContext(ctx context.Context, edgeFilter func(from, to *Value) bool, next func(*Value) Values, end ...*Value) []Values {
	paths := make([]Values, 0)
	v.visitPathsWithDirection(ctx, edgeFilter, next, nil, func(path Values) bool {
		paths = append(paths, append(Values{}, path...))
		return true
	}, end...)
	return paths
}

// The visitor receives a borrowed path without the root. Only active-path keys
// close cycles: a global visited set would discard distinct branch continuations.
func (v *Value) visitPathsWithDirection(ctx context.Context, edgeFilter func(from, to *Value) bool, next func(*Value) Values, bail func() bool, visit func(Values) bool, end ...*Value) bool {
	active := make(map[valueGraphKey]bool)
	var path Values
	var walk func(*Value) bool
	walk = func(node *Value) bool {
		if ctx.Err() != nil || (bail != nil && bail()) {
			return false
		}
		if node == nil {
			return true
		}
		key := node.graphKey()
		if active[key] {
			return true
		}
		active[key] = true
		path = append(path, node)
		defer func() { delete(active, key); path = path[:len(path)-1] }()
		var neighbors Values
		if !ValueContain(node, end...) {
			seen := make(map[valueGraphKey]bool)
			for _, child := range next(node) {
				if ctx.Err() != nil {
					return false
				}
				if child == nil || (edgeFilter != nil && !edgeFilter(node, child)) {
					continue
				}
				childKey := child.graphKey()
				if !seen[childKey] {
					seen[childKey] = true
					neighbors = append(neighbors, child)
				}
			}
		}
		if ctx.Err() != nil {
			return false
		}
		if len(neighbors) == 0 {
			withoutRoot := path[1:]
			if len(end) == 0 {
				return visit(withoutRoot)
			}
			for _, target := range end {
				if ValueContain(target, withoutRoot...) {
					return visit(withoutRoot)
				}
			}
			return true
		}
		for _, child := range neighbors {
			if !walk(child) {
				return false
			}
		}
		return true
	}
	return walk(v)
}

func (vs Values) GetDataflowPath(end ...*Value) []Values {
	path := make([]Values, 0)
	for _, v := range vs {
		path = append(path, v.GetDataflowPath(end...)...)
	}
	return path
}
