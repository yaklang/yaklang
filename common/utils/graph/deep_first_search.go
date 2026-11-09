package graph

import (
	"context"
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

	// visit receives every completed path. Returning false stops the whole walk
	// immediately (used by dataflow filtering to stop at the first accepted path
	// instead of materializing every path).
	visit func([]U) bool
	// prune, when set, is asked about every child before descending into it.
	// Returning true drops that child subtree. A node whose children are all
	// pruned is NOT emitted as a leaf: the node had outgoing edges, so the
	// original walk would not have ended there.
	prune func(T) bool

	// deep first search stack
	current *omap.OrderedMap[K, U]
	ctx     context.Context
}

// deepFirst returns false only when the walk must stop, either because the
// visitor declined the last path or because ctx was cancelled.
func (d *DeepFirstPath[K, T, U]) deepFirst(node T, target ...T) bool {
	select {
	case <-d.ctx.Done():
		return false
	default:
	}

	key := d.getKey(node)
	if d.current.Have(key) {
		return true
	}
	d.current.PushKey(key, d.getValue(node))
	defer d.current.Pop()

	nextNodes := d.next(node)
	if len(nextNodes) > 1 {
		nextNodes = lo.UniqBy(nextNodes, d.getKey)
	}

	if len(target) > 0 {
		// A target node ends the path even when it has outgoing edges.
		if slices.ContainsFunc(target, func(t T) bool {
			return d.getKey(t) == key
		}) {
			return d.visit(d.current.Values())
		}
	} else if len(nextNodes) == 0 {
		return d.visit(d.current.Values())
	}

	if d.prune != nil {
		// nextNodes comes from next (or lo.UniqBy), so compacting it in place
		// does not alias caller state.
		kept := nextNodes[:0]
		for _, next := range nextNodes {
			if !d.prune(next) {
				kept = append(kept, next)
			}
		}
		if len(kept) == 0 {
			// Every outgoing branch was pruned. This is not a real leaf, so do
			// not emit a shortened path that the original walk never produced.
			return true
		}
		nextNodes = kept
	}

	if len(nextNodes) == 1 {
		return d.deepFirst(nextNodes[0], target...)
	}

	for _, next := range nextNodes {
		if !d.deepFirst(next, target...) {
			return false
		}
	}
	return true
}

func GraphPathEx[K comparable, T, U any](
	ctx context.Context,
	node T,
	next func(T) []T,
	getKey func(T) K,
	getValue func(T) U,
	target ...T,
) [][]U {
	res := make([][]U, 0)
	WalkGraphPathEx(ctx, node, next, getKey, getValue, nil, func(path []U) bool {
		res = append(res, path)
		return true
	}, target...)
	return res
}

// WalkGraphPathEx is the streaming form of [GraphPathEx]. visit is called with
// each completed path in DFS order; returning false stops the walk at once.
// prune, when non-nil, drops a child subtree (and prevents the parent from
// being reported as a leaf). Returns false when the walk was stopped early
// (by ctx cancellation or by visit), true when it completed.
func WalkGraphPathEx[K comparable, T, U any](
	ctx context.Context,
	node T,
	next func(T) []T,
	getKey func(T) K,
	getValue func(T) U,
	prune func(T) bool,
	visit func([]U) bool,
	target ...T,
) bool {
	df := &DeepFirstPath[K, T, U]{
		ctx:      ctx,
		current:  omap.NewEmptyOrderedMap[K, U](),
		next:     next,
		getKey:   getKey,
		getValue: getValue,
		prune:    prune,
		visit:    visit,
	}
	return df.deepFirst(node, target...)
}

// index by K type, and return path with T type
func GraphPathWithKey[K comparable, T any](
	node T,
	next func(T) []T,
	getKey func(T) K,
) [][]T {
	return GraphPathEx(context.Background(), node, next, getKey, func(t T) T { return t })
}

// index by T type, and return path with U type
func GraphPathWithValue[T comparable, U any](
	node T,
	next func(T) []T,
	getValue func(T) U,
) [][]U {
	return GraphPathEx(context.Background(), node, next, func(t T) T { return t }, getValue)
}

// index by T type , and return path with T type
func GraphPath[T comparable](
	node T,
	next func(T) []T,
) [][]T {
	return GraphPathEx(context.Background(), node, next,
		func(t T) T { return t },
		func(t T) T { return t },
	)
}

func GraphPathWithTarget[T comparable](
	ctx context.Context,
	node T,
	target T,
	next func(T) []T,
) [][]T {
	return GraphPathEx[T, T, T](ctx, node, next,
		func(t T) T { return t },
		func(t T) T { return t },
		target,
	)
}
