package graph

import (
	"slices"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// deep first search for nodeID and its children to [][]id, id is string,
// if node.Prev have more than one, add a new line
type DeepFirstPath[K comparable, T, U any] struct {

	// graph
	next func(T) []T

	// data
	getKey   func(T) K
	getValue func(T) U

	// value result
	res [][]U

	// deep first search stack
	current *omap.OrderedMap[K, U]
}

func (d *DeepFirstPath[K, T, U]) deepFirst(node T, target ...T) {
	key := d.getKey(node)
	value := d.getValue(node)

	if d.current.Have(key) {
		// log.Infof("node %v already in current skip", node)
		return
	}
	d.current.PushKey(key, value)
	defer d.current.Pop()

	// log.Infof("node %v add to current path: %v", node, d.current.Keys())
	nextNodes := d.next(node)
	nextNodes = lo.UniqBy(nextNodes, d.getKey)
	// log.Infof("next node :%v", nextNodes)
	if len(target) > 0 {
		// if have target, check if current node is target
		if slices.ContainsFunc(target, func(t T) bool {
			return d.getKey(t) == key
		}) {
			// if current node is target, add to result
			d.res = append(d.res, d.current.Values())
			return
		}
	} else {
		// if not target, check if current node is end
		if len(nextNodes) == 0 {
			// the end
			d.res = append(d.res, d.current.Values())
			return
		}
	}

	if len(nextNodes) == 1 {
		prev := nextNodes[0]
		d.deepFirst(prev, target...)
		return
	}

	for _, next := range nextNodes {
		d.deepFirst(next, target...)
	}
}

func GraphPathEx[K comparable, T, U any](
	node T,
	next func(T) []T,
	getKey func(T) K,
	getValue func(T) U,
	target ...T,
) [][]U {
	df := &DeepFirstPath[K, T, U]{
		res:      make([][]U, 0),
		current:  omap.NewEmptyOrderedMap[K, U](),
		next:     next,
		getKey:   getKey,
		getValue: getValue,
	}
	df.deepFirst(node, target...)
	return df.res
}

// index by K type, and return path with T type
func GraphPathWithKey[K comparable, T any](
	node T,
	next func(T) []T,
	getKey func(T) K,
) [][]T {
	return GraphPathEx(node, next, getKey, func(t T) T { return t })
}

// index by T type, and return path with U type
func GraphPathWithValue[T comparable, U any](
	node T,
	next func(T) []T,
	getValue func(T) U,
) [][]U {
	return GraphPathEx(node, next, func(t T) T { return t }, getValue)
}

// index by T type , and return path with T type
func GraphPath[T comparable](
	node T,
	next func(T) []T,
) [][]T {
	return GraphPathEx(node, next,
		func(t T) T { return t },
		func(t T) T { return t },
	)
}

func GraphPathWithTarget[T comparable](
	node T,
	target T,
	next func(T) []T,
) [][]T {
	return GraphPathEx[T, T, T](node, next,
		func(t T) T { return t },
		func(t T) T { return t },
		target,
	)
}
