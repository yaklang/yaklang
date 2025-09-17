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
	return v.getPathWithDirection(
		func(node *Value) Values {
			return node.GetEffectOn()
		},
		end...,
	)
}

func (v *Value) GetDependOnPath(end ...*Value) []Values {
	return v.getPathWithDirection(
		func(node *Value) Values {
			return node.GetDependOn()
		},
		end...,
	)
}

func (this *Value) getPathWithDirection(next func(*Value) Values, end ...*Value) []Values {
	ret := graph.GraphPathWithKey[int64, *Value](
		this,
		func(node *Value) []*Value { // next
			if ValueContain(node, end...) {
				return nil
			}
			return next(node) // todo: handler endValue in here
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
