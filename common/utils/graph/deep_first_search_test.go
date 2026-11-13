package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalkGraphPathExStopsAtFirstAcceptedPath(t *testing.T) {
	next := func(n int) []int {
		switch n {
		case 1:
			return []int{2, 3}
		case 2:
			return []int{4}
		}
		return nil
	}

	var all [][]int
	complete := WalkGraphPathEx[int, int, int](
		context.Background(), 1, next,
		func(n int) int { return n },
		func(n int) int { return n },
		nil,
		func(path []int) bool {
			all = append(all, append([]int(nil), path...))
			return true
		},
	)
	require.True(t, complete, "a visitor that never stops must complete the walk")
	require.Len(t, all, 2)

	visited := 0
	complete = WalkGraphPathEx[int, int, int](
		context.Background(), 1, next,
		func(n int) int { return n },
		func(n int) int { return n },
		nil,
		func(path []int) bool {
			visited++
			return false
		},
	)
	require.False(t, complete, "a visitor returning false stops the walk")
	require.Equal(t, 1, visited, "the DFS must stop at the first accepted path")
}

func TestWalkGraphPathExPrunedChildrenAreNotALeaf(t *testing.T) {
	next := func(n int) []int {
		if n == 1 {
			return []int{2, 3}
		}
		return nil
	}
	pruneAll := func(int) bool { return true }

	var paths [][]int
	complete := WalkGraphPathEx[int, int, int](
		context.Background(), 1, next,
		func(n int) int { return n },
		func(n int) int { return n },
		pruneAll,
		func(path []int) bool {
			paths = append(paths, append([]int(nil), path...))
			return true
		},
	)
	require.True(t, complete)
	require.Empty(t, paths, "a node whose children were all pruned must not be emitted as a shortened leaf")
}

// next is allowed to return a slice the caller still owns (dot graphs expose
// node adjacency directly), so dedup and pruning must never compact it in place.
func TestWalkGraphPathExLeavesNextSliceIntact(t *testing.T) {
	nextWith := func(children []int) func(int) []int {
		return func(n int) []int {
			if n == 1 {
				return children
			}
			return nil
		}
	}

	t.Run("dedup", func(t *testing.T) {
		children := []int{2, 2, 3}
		var got [][]int
		complete := WalkGraphPathEx[int, int, int](
			context.Background(), 1, nextWith(children),
			func(n int) int { return n },
			func(n int) int { return n },
			nil,
			func(path []int) bool {
				got = append(got, append([]int(nil), path...))
				return true
			},
		)
		require.True(t, complete)
		require.Equal(t, [][]int{{1, 2}, {1, 3}}, got)
		require.Equal(t, []int{2, 2, 3}, children)
	})

	t.Run("prune", func(t *testing.T) {
		children := []int{2, 3}
		var got [][]int
		complete := WalkGraphPathEx[int, int, int](
			context.Background(), 1, nextWith(children),
			func(n int) int { return n },
			func(n int) int { return n },
			func(n int) bool { return n == 3 },
			func(path []int) bool {
				got = append(got, append([]int(nil), path...))
				return true
			},
		)
		require.True(t, complete)
		require.Equal(t, [][]int{{1, 2}}, got)
		require.Equal(t, []int{2, 3}, children)
	})
}
