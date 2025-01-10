package graph

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
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
	current *orderedmap.OrderedMapEx[K, U] // map[string]nil
}

func (d *DeepFirstPath[K, T, U]) deepFirst(node T) {
	key := d.getKey(node)
	value := d.getValue(node)
	if _, ok := d.current.Get(key); ok {
		// log.Infof("node %d already in current skip", nodeID)
		return
	}
	d.current.Set(key, value)
	// log.Infof("node %d add to current path: %v", nodeID, d.current.Keys())
	nextNodes := d.next(node)
	nextNodes = lo.UniqBy(nextNodes, d.getKey)
	// log.Infof("next node :%v", nextNodes)
	if len(nextNodes) == 0 {
		d.res = append(d.res, d.current.Values())
		return
	}
	if len(nextNodes) == 1 {
		prev := nextNodes[0]
		d.deepFirst(prev)
		return
	}

	// origin
	current := d.current
	for _, next := range nextNodes {
		// new line
		d.current = current.Copy()
		d.deepFirst(next)
	}
}

func GraphPathEx[K comparable, T, U any](
	node T,
	next func(T) []T,
	getKey func(T) K,
	getValue func(T) U,
) [][]U {
	df := &DeepFirstPath[K, T, U]{
		res:      make([][]U, 0),
		current:  orderedmap.NewOrderMapEx[K, U](nil, nil, false),
		next:     next,
		getKey:   getKey,
		getValue: getValue,
	}
	df.deepFirst(node)
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
