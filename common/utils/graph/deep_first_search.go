package graph

import (
	"context"
	"slices"
)

// deepFirstDedupLinearMax is the largest child count deduped with a linear
// scan. Out-degrees in call/dataflow graphs are small, so this avoids the map
// allocation on nearly every branching node.
const deepFirstDedupLinearMax = 8

// DeepFirstPath enumerates every simple path (no repeated key) from the start
// node to a leaf or to one of the target nodes.
type DeepFirstPath[K comparable, T, U any] struct {

	// graph
	next func(T) []T

	// data
	getKey   func(T) K
	getValue func(T) U

	// visit receives every completed path. Returning false stops the whole walk
	// immediately (used by dataflow filtering to stop at the first accepted path
	// instead of materializing every path). The slice is owned by the caller.
	visit func([]U) bool
	// prune, when set, is asked about every child before descending into it.
	// Returning true drops that child subtree. A node whose children are all
	// pruned is NOT emitted as a leaf: the node had outgoing edges, so the
	// original walk would not have ended there.
	prune func(T) bool

	// Deep-first stack. The walk is single-goroutine, so a plain slice plus a
	// membership map replaces the ordered map used before: that one took an
	// RWMutex per operation and Pop scanned the whole key chain.
	stack []U
	seen  map[K]struct{}

	// target is non-nil when the walk should end at the given keys instead of
	// at leaves.
	target map[K]struct{}

	done <-chan struct{}
}

// deepFirst returns false only when the walk must stop, either because the
// visitor declined the last path or because ctx was cancelled.
func (d *DeepFirstPath[K, T, U]) deepFirst(node T) bool {
	select {
	case <-d.done:
		return false
	default:
	}

	key := d.getKey(node)
	if _, ok := d.seen[key]; ok {
		return true
	}
	d.seen[key] = struct{}{}
	d.stack = append(d.stack, d.getValue(node))
	cont := d.deepFirstInner(node, key)
	d.stack = d.stack[:len(d.stack)-1]
	delete(d.seen, key)
	return cont
}

func (d *DeepFirstPath[K, T, U]) deepFirstInner(node T, key K) bool {
	if len(d.target) > 0 {
		// A target node ends the path even when it has outgoing edges.
		if _, ok := d.target[key]; ok {
			return d.visit(slices.Clone(d.stack))
		}
	}

	nextNodes := d.next(node)
	if len(nextNodes) > 1 {
		nextNodes = dedupByKey(nextNodes, d.getKey)
	}

	if len(d.target) == 0 && len(nextNodes) == 0 {
		return d.visit(slices.Clone(d.stack))
	}

	if d.prune != nil {
		// next may return a slice owned by the caller (dot graphs expose node
		// adjacency directly), so only build a filtered slice when something is
		// actually dropped.
		first := -1
		for i, next := range nextNodes {
			if d.prune(next) {
				first = i
				break
			}
		}
		if first >= 0 {
			kept := make([]T, 0, len(nextNodes)-1)
			kept = append(kept, nextNodes[:first]...)
			for _, next := range nextNodes[first+1:] {
				if !d.prune(next) {
					kept = append(kept, next)
				}
			}
			if len(kept) == 0 {
				// Every outgoing branch was pruned. This is not a real leaf, so
				// do not emit a shortened path that the original walk never
				// produced.
				return true
			}
			nextNodes = kept
		}
	}

	if len(nextNodes) == 1 {
		return d.deepFirst(nextNodes[0])
	}

	for _, next := range nextNodes {
		if !d.deepFirst(next) {
			return false
		}
	}
	return true
}

// dedupByKey drops repeated keys while preserving first-seen order. next may
// return a slice the caller still owns, so it only allocates when a duplicate
// is actually present.
func dedupByKey[K comparable, T any](nodes []T, getKey func(T) K) []T {
	if len(nodes) < 2 {
		return nodes
	}

	duplicate := -1
	if len(nodes) <= deepFirstDedupLinearMax {
		for i := 1; i < len(nodes) && duplicate < 0; i++ {
			key := getKey(nodes[i])
			for j := 0; j < i; j++ {
				if getKey(nodes[j]) == key {
					duplicate = i
					break
				}
			}
		}
	} else {
		seen := make(map[K]struct{}, len(nodes))
		for i, node := range nodes {
			key := getKey(node)
			if _, ok := seen[key]; ok {
				duplicate = i
				break
			}
			seen[key] = struct{}{}
		}
	}
	if duplicate < 0 {
		return nodes
	}

	out := make([]T, 0, len(nodes))
	seen := make(map[K]struct{}, len(nodes))
	for _, node := range nodes {
		key := getKey(node)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, node)
	}
	return out
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
	var targetKeys map[K]struct{}
	if len(target) > 0 {
		targetKeys = make(map[K]struct{}, len(target))
		for _, t := range target {
			targetKeys[getKey(t)] = struct{}{}
		}
	}
	df := &DeepFirstPath[K, T, U]{
		done:     ctx.Done(),
		stack:    make([]U, 0, 16),
		seen:     make(map[K]struct{}),
		target:   targetKeys,
		next:     next,
		getKey:   getKey,
		getValue: getValue,
		prune:    prune,
		visit:    visit,
	}
	return df.deepFirst(node)
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
