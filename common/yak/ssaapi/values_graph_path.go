package ssaapi

import (
	"context"

	"github.com/yaklang/yaklang/common/utils/graph"
)

// GetDataflowPath collects every dataflow path. Prefer
// [Value.WalkDataflowPathWithContext] when the caller can stop at the first
// accepted path: this form materializes all of them.
func (v *Value) GetDataflowPath(end ...*Value) []Values {
	var paths []Values
	v.WalkDataflowPathWithContext(context.Background(), func(path Values) bool {
		paths = append(paths, path)
		return true
	}, end...)
	return paths
}

// GetDataflowPathWithContext has no caller since the dataflow engine switched
// to streaming (dataFlowFilter -> walkDataflowPathWithContext directly).
// Commented out rather than deleted as requested.
//
// func (v *Value) GetDataflowPathWithContext(ctx context.Context, end ...*Value) []Values {
// 	var paths []Values
// 	v.walkDataflowPathWithContext(ctx, nil, nil, func(path Values) bool {
// 		paths = append(paths, path)
// 		return true
// 	}, false, end...)
// 	return paths
// }

// WalkDataflowPathWithContext is the streaming form of
// [GetDataflowPathWithContext]. visit receives each dataflow path (effect ->
// center -> depend) as it is completed; returning false stops the enumeration
// immediately. This lets dataflow(include/exclude) stop at the first accepted
// path instead of materializing every effect×depend combination.
func (v *Value) WalkDataflowPathWithContext(ctx context.Context, visit func(Values) bool, end ...*Value) bool {
	return v.walkDataflowPathWithContext(ctx, nil, nil, visit, false, end...)
}

// WalkDataflowPathWithEdgeFilterWithContext is the edge-filtered streaming
// form of [GetDataflowPathWithEdgeFilterWithContext].
func (v *Value) WalkDataflowPathWithEdgeFilterWithContext(
	ctx context.Context,
	edgeFilter func(from, to *Value) bool,
	visit func(Values) bool,
	end ...*Value,
) bool {
	return v.walkDataflowPathWithContext(ctx, edgeFilter, nil, visit, false, end...)
}

// walkDataflowPathWithContext streams effect×depend paths without building the
// cross product. prune, when non-nil, drops a branch subtree during DFS (used
// by dataflow exclude pruning). reusePath promises that visit does not retain
// the Values slice it receives, which lets the effect×depend combination reuse
// one buffer instead of allocating a path per pair.
func (v *Value) walkDataflowPathWithContext(
	ctx context.Context,
	edgeFilter func(from, to *Value) bool,
	prune func(*Value) bool,
	visit func(Values) bool,
	reusePath bool,
	end ...*Value,
) bool {
	if v == nil {
		return true
	}

	effectNext := func(node *Value) Values { return node.GetEffectOn() }
	dependNext := func(node *Value) Values { return node.GetDependOn() }

	// A direction that originally had outgoing edges but produced no path was
	// fully pruned. The old enumerator would have produced paths through those
	// edges (all rejected), so the "other direction only" fallback below must
	// stay disabled; otherwise pruning would invent a shorter path that never
	// existed.
	effectHadChildren := v.walkDirectionHasChildren(edgeFilter, effectNext, end...)
	dependHadChildren := v.walkDirectionHasChildren(edgeFilter, dependNext, end...)

	var effectPaths []Values
	if !v.walkPathWithDirectionWithContext(ctx, edgeFilter, prune, func(path Values) bool {
		effectPaths = append(effectPaths, path)
		return true
	}, effectNext, end...) {
		return false
	}
	if len(effectPaths) == 0 && effectHadChildren {
		// Every effect branch was pruned; no combined path can survive.
		return true
	}

	dependCount := 0
	var dependVisit func(Values) bool
	if reusePath {
		// Effect paths are retained, so only the combination is reused here.
		buf := make(Values, 0, 16)
		dependVisit = func(depend Values) bool {
			dependCount++
			if len(effectPaths) == 0 {
				buf = append(buf[:0], v)
				buf = append(buf, depend...)
				return visit(buf)
			}
			for _, effect := range effectPaths {
				buf = append(buf[:0], effect...)
				buf = append(buf, v)
				buf = append(buf, depend...)
				if !visit(buf) {
					return false
				}
			}
			return true
		}
	} else {
		dependVisit = func(depend Values) bool {
			dependCount++
			if len(effectPaths) == 0 {
				return visit(v.makeDataflowPath(Values{}, depend))
			}
			for _, effect := range effectPaths {
				if !visit(v.makeDataflowPath(effect, depend)) {
					return false
				}
			}
			return true
		}
	}

	completed := v.walkPathWithDirectionWithContext(ctx, edgeFilter, prune, dependVisit, dependNext, end...)
	if !completed {
		return false
	}
	if dependCount == 0 && !dependHadChildren {
		if reusePath {
			buf := make(Values, 0, 16)
			for _, effect := range effectPaths {
				buf = append(buf[:0], effect...)
				buf = append(buf, v)
				if !visit(buf) {
					return false
				}
			}
			return true
		}
		for _, effect := range effectPaths {
			if !visit(v.makeDataflowPath(effect, Values{})) {
				return false
			}
		}
	}
	return true
}

func (v *Value) makeDataflowPath(effect, depend Values) Values {
	path := make(Values, 0, len(effect)+len(depend)+1)
	path = append(path, effect...)
	path = append(path, v)
	path = append(path, depend...)
	return path
}

// walkDirectionHasChildren reports whether a directional walk from v would
// start with at least one child (i.e. v is not itself an end target and the
// edge filter keeps a neighbor).
func (v *Value) walkDirectionHasChildren(
	edgeFilter func(from, to *Value) bool,
	next func(*Value) Values,
	end ...*Value,
) bool {
	if ValueContain(v, end...) {
		return false
	}
	for _, child := range next(v) {
		if child == nil {
			continue
		}
		if edgeFilter == nil || edgeFilter(v, child) {
			return true
		}
	}
	return false
}

func (v *Value) GetEffectOnPath(end ...*Value) []Values {
	return v.GetEffectOnPathWithContext(context.Background(), end...)
}
func (v *Value) GetEffectOnPathWithContext(ctx context.Context, end ...*Value) []Values {
	return v.getPathWithDirectionWithContext(ctx, nil, func(node *Value) Values {
		return node.GetEffectOn()
	}, end...)
}

func (v *Value) GetDependOnPath(end ...*Value) []Values {
	return v.GetDependOnPathWithContext(context.Background(), end...)
}
func (v *Value) GetDependOnPathWithContext(ctx context.Context, end ...*Value) []Values {
	return v.getPathWithDirectionWithContext(ctx, nil, func(node *Value) Values {
		return node.GetDependOn()
	}, end...)
}

// GetEffectOnPathWithEdgeFilter is like [GetEffectOnPath] but drops edges (from→to) when
// edgeFilter returns false. A nil edgeFilter disables filtering.
func (v *Value) GetEffectOnPathWithEdgeFilter(edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.GetEffectOnPathWithEdgeFilterWithContext(context.Background(), edgeFilter, end...)
}
func (v *Value) GetEffectOnPathWithEdgeFilterWithContext(ctx context.Context, edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.getPathWithDirectionWithContext(ctx, edgeFilter, func(node *Value) Values {
		return node.GetEffectOn()
	}, end...)
}

// GetDependOnPathWithEdgeFilter is like [GetDependOnPath] but drops edges when edgeFilter returns false.
func (v *Value) GetDependOnPathWithEdgeFilter(edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.GetDependOnPathWithEdgeFilterWithContext(context.Background(), edgeFilter, end...)
}
func (v *Value) GetDependOnPathWithEdgeFilterWithContext(ctx context.Context, edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.getPathWithDirectionWithContext(ctx, edgeFilter, func(node *Value) Values {
		return node.GetDependOn()
	}, end...)
}

func (v *Value) GetDataflowPathWithEdgeFilter(edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.GetDataflowPathWithEdgeFilterWithContext(context.Background(), edgeFilter, end...)
}
func (v *Value) GetDataflowPathWithEdgeFilterWithContext(ctx context.Context, edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	var paths []Values
	v.walkDataflowPathWithContext(ctx, edgeFilter, nil, func(path Values) bool {
		paths = append(paths, path)
		return true
	}, false, end...)
	return paths
}

// getPathWithDirectionWithContext drives the DFS with ctx. WalkGraphPathEx
// already checks ctx.Done() at every node, so a cancelled rule ctx stops the
// enumeration.
func (this *Value) getPathWithDirectionWithContext(ctx context.Context, edgeFilter func(from, to *Value) bool, next func(*Value) Values, end ...*Value) []Values {
	paths := make([]Values, 0)
	this.walkPathWithDirectionWithContext(ctx, edgeFilter, nil, func(path Values) bool {
		paths = append(paths, path)
		return true
	}, next, end...)
	return paths
}

// walkPathWithDirectionWithContext streams one direction's DFS paths. The
// starting value is removed from each path and, when end is set, only paths
// containing an end value are delivered (matching the slice-returning form).
func (this *Value) walkPathWithDirectionWithContext(
	ctx context.Context,
	edgeFilter func(from, to *Value) bool,
	prune func(*Value) bool,
	visit func(Values) bool,
	next func(*Value) Values,
	end ...*Value,
) bool {
	return graph.WalkGraphPathEx[int64, *Value, *Value](
		ctx,
		this,
		func(node *Value) []*Value { // next
			if ValueContain(node, end...) {
				return nil
			}
			neighbors := next(node)
			if edgeFilter == nil {
				return neighbors
			}
			out := make([]*Value, 0, len(neighbors))
			for _, succ := range neighbors {
				if succ != nil && edgeFilter(node, succ) {
					out = append(out, succ)
				}
			}
			return out
		},
		func(node *Value) int64 { // getKey
			return node.GetId()
		},
		func(t *Value) *Value { // getValue
			return t
		},
		prune,
		func(path []*Value) bool {
			// The DFS pushes the start value first, so dropping it is a reslice
			// rather than a search-and-shift over the path.
			path = path[1:]
			if len(end) == 0 {
				return visit(path)
			}
			for _, end := range end {
				if ValueContain(end, path...) {
					return visit(path)
				}
			}
			return true
		},
		end...,
	)
}

func (vs Values) GetDataflowPath(end ...*Value) []Values {
	path := make([]Values, 0)
	for _, v := range vs {
		path = append(path, v.GetDataflowPath(end...)...)
	}
	return path
}
