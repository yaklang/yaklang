package ssaapi

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/graph"
)

func (v *Value) GetDataflowPath(end ...*Value) []Values {
	return v.GetDataflowPathWithContext(context.Background(), end...)
}

// GetDataflowPathWithContext is like [GetDataflowPath] but the DFS respects ctx:
// when ctx is cancelled the path enumeration bails early (see graph.GraphPathEx /
// DeepFirstPath.deepFirst). This is what lets the per-rule wall-clock budget
// (syntaxflow_scan/runtime.go Query -> QueryWithContext -> sfvm.Config.ctx) reach
// the heavy path enumeration that dataflow(include=...) triggers on large
// projects. Without this, GraphPathWithKey used context.Background() and the
// per-rule deadline never fired — heavy rules ran for hours.
func (v *Value) GetDataflowPathWithContext(ctx context.Context, end ...*Value) []Values {
	var paths []Values
	effectPath := v.GetEffectOnPathWithContext(ctx, end...)
	dependPath := v.GetDependOnPathWithContext(ctx, end...)
	addPath := func(effect, depend Values) {
		path := make(Values, 0, len(effect)+len(depend)+1)
		path = append(path, effect...)
		path = append(path, v)
		path = append(path, depend...)
		paths = append(paths, path)
	}

	if len(effectPath) == 0 { // if no effect, then depend is the path
		for _, depend := range dependPath {
			addPath(Values{}, depend)
		}
	}
	if len(dependPath) == 0 { // if no depend, then effect is the path
		for _, effect := range effectPath {
			addPath(effect, Values{})
		}
	}
	for _, effect := range effectPath {
		for _, depend := range dependPath {
			addPath(effect, depend) // effect -> v -> depend
		}
	}
	return paths
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
	effectPath := v.GetEffectOnPathWithEdgeFilterWithContext(ctx, edgeFilter, end...)
	dependPath := v.GetDependOnPathWithEdgeFilterWithContext(ctx, edgeFilter, end...)
	addPath := func(effect, depend Values) {
		path := make(Values, 0, len(effect)+len(depend)+1)
		path = append(path, effect...)
		path = append(path, v)
		path = append(path, depend...)
		paths = append(paths, path)
	}

	if len(effectPath) == 0 {
		for _, depend := range dependPath {
			addPath(Values{}, depend)
		}
	}
	if len(dependPath) == 0 {
		for _, effect := range effectPath {
			addPath(effect, Values{})
		}
	}
	for _, effect := range effectPath {
		for _, depend := range dependPath {
			addPath(effect, depend)
		}
	}
	return paths
}

// getPathWithDirectionWithContext drives the DFS with ctx. GraphPathEx already
// checks ctx.Done() at every node (DeepFirstPath.deepFirst), so a cancelled
// rule ctx stops the enumeration. GraphPathWithKey (the old caller) hard-coded
// context.Background(), which is why the per-rule budget never bounded it.
func (this *Value) getPathWithDirectionWithContext(ctx context.Context, edgeFilter func(from, to *Value) bool, next func(*Value) Values, end ...*Value) []Values {
	ret := graph.GraphPathEx[int64, *Value, *Value](
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
	)
	paths := make([]Values, 0, len(ret))
	for _, path := range ret {
		path = utils.RemoveSliceItem(path, this)
		if len(end) == 0 {
			paths = append(paths, path)
			continue
		}
		for _, end := range end {
			if ValueContain(end, path...) {
				paths = append(paths, path)
				break
			}
		}
	}
	return paths
}

func (vs Values) GetDataflowPath(end ...*Value) []Values {
	path := make([]Values, 0)
	for _, v := range vs {
		path = append(path, v.GetDataflowPath(end...)...)
	}
	return path
}
