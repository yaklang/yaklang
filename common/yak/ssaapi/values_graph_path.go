package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/graph"
)

func (v *Value) GetDataflowPath(end ...*Value) []Values {
	var paths []Values
	effectPath := v.GetEffectOnPath(end...)
	dependPath := v.GetDependOnPath(end...)
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
	return v.getPathWithDirection(nil, func(node *Value) Values {
		return node.GetEffectOn()
	}, end...)
}

func (v *Value) GetDependOnPath(end ...*Value) []Values {
	return v.getPathWithDirection(nil, func(node *Value) Values {
		return node.GetDependOn()
	}, end...)
}

// GetEffectOnPathWithEdgeFilter is like [GetEffectOnPath] but drops edges (from→to) when
// edgeFilter returns false. A nil edgeFilter disables filtering.
func (v *Value) GetEffectOnPathWithEdgeFilter(edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.getPathWithDirection(edgeFilter, func(node *Value) Values {
		return node.GetEffectOn()
	}, end...)
}

// GetDependOnPathWithEdgeFilter is like [GetDependOnPath] but drops edges when edgeFilter returns false.
func (v *Value) GetDependOnPathWithEdgeFilter(edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	return v.getPathWithDirection(edgeFilter, func(node *Value) Values {
		return node.GetDependOn()
	}, end...)
}

func (v *Value) GetDataflowPathWithEdgeFilter(edgeFilter func(from, to *Value) bool, end ...*Value) []Values {
	var paths []Values
	effectPath := v.GetEffectOnPathWithEdgeFilter(edgeFilter, end...)
	dependPath := v.GetDependOnPathWithEdgeFilter(edgeFilter, end...)
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

func (this *Value) getPathWithDirection(edgeFilter func(from, to *Value) bool, next func(*Value) Values, end ...*Value) []Values {
	ret := graph.GraphPathWithKey[int64, *Value](
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
