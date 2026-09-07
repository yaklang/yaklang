package sfreport

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeDataFlowPathsBasic(t *testing.T) {
	t.Run("empty nodes", func(t *testing.T) {
		require.Nil(t, computeDataFlowPaths(nil, nil))
	})

	t.Run("single entry node", func(t *testing.T) {
		paths := computeDataFlowPaths(
			[]*NodeInfo{{NodeID: "n1", IsEntryNode: true}},
			nil,
		)
		assert.Equal(t, [][]string{{"n1"}}, paths)
	})

	t.Run("linear chain", func(t *testing.T) {
		nodes := chainNodes(3)
		nodes[0].IsEntryNode = true
		edges := []*EdgeInfo{
			{EdgeID: "e1", FromNodeID: "n0", ToNodeID: "n1"},
			{EdgeID: "e2", FromNodeID: "n1", ToNodeID: "n2"},
		}
		assert.Equal(t, [][]string{{"n0", "n1", "n2"}}, computeDataFlowPaths(nodes, edges))
	})

	t.Run("branch and merge", func(t *testing.T) {
		nodes := chainNodes(4)
		nodes[0].IsEntryNode = true
		edges := []*EdgeInfo{
			{EdgeID: "e1", FromNodeID: "n0", ToNodeID: "n1"},
			{EdgeID: "e2", FromNodeID: "n0", ToNodeID: "n2"},
			{EdgeID: "e3", FromNodeID: "n1", ToNodeID: "n3"},
			{EdgeID: "e4", FromNodeID: "n2", ToNodeID: "n3"},
		}
		paths := computeDataFlowPaths(nodes, edges)
		assert.ElementsMatch(t, [][]string{{"n0", "n1", "n3"}, {"n0", "n2", "n3"}}, paths)
	})

	t.Run("cycle does not loop forever", func(t *testing.T) {
		nodes := chainNodes(3)
		nodes[0].IsEntryNode = true
		edges := []*EdgeInfo{
			{EdgeID: "e1", FromNodeID: "n0", ToNodeID: "n1"},
			{EdgeID: "e2", FromNodeID: "n1", ToNodeID: "n0"},
			{EdgeID: "e3", FromNodeID: "n1", ToNodeID: "n2"},
		}
		assert.Equal(t, [][]string{{"n0", "n1", "n2"}}, computeDataFlowPaths(nodes, edges))
	})

	t.Run("nil entries are skipped", func(t *testing.T) {
		nodes := []*NodeInfo{nil, {NodeID: "n0", IsEntryNode: true}, {NodeID: "n1"}}
		edges := []*EdgeInfo{nil, {EdgeID: "e1", FromNodeID: "n0", ToNodeID: "n1"}}
		assert.Equal(t, [][]string{{"n0", "n1"}}, computeDataFlowPaths(nodes, edges))
	})

	t.Run("falls back to first node", func(t *testing.T) {
		nodes := chainNodes(2)
		paths := computeDataFlowPaths(nodes, nil)
		assert.Equal(t, [][]string{{"n0"}}, paths)
	})
}

func TestComputeDataFlowPaths_CapsPathCount(t *testing.T) {
	// Full binary tree with 13 levels would produce 4096 simple paths;
	// the cap must stop enumeration at maxDataFlowPathCount.
	depth := 12
	total := 1<<(depth+1) - 1
	nodes := make([]*NodeInfo, total)
	for i := range nodes {
		nodes[i] = &NodeInfo{NodeID: fmt.Sprintf("n%d", i)}
	}
	nodes[0].IsEntryNode = true

	edges := make([]*EdgeInfo, 0, total-1)
	for i := 0; i < total/2; i++ {
		left := 2*i + 1
		right := 2*i + 2
		edges = append(edges,
			&EdgeInfo{EdgeID: fmt.Sprintf("e%d-%d", i, left), FromNodeID: fmt.Sprintf("n%d", i), ToNodeID: fmt.Sprintf("n%d", left)},
			&EdgeInfo{EdgeID: fmt.Sprintf("e%d-%d", i, right), FromNodeID: fmt.Sprintf("n%d", i), ToNodeID: fmt.Sprintf("n%d", right)},
		)
	}

	validEdges := make(map[string]map[string]bool, len(edges))
	for _, e := range edges {
		if validEdges[e.FromNodeID] == nil {
			validEdges[e.FromNodeID] = make(map[string]bool)
		}
		validEdges[e.FromNodeID][e.ToNodeID] = true
	}

	paths := computeDataFlowPaths(nodes, edges)
	require.Len(t, paths, maxDataFlowPathCount)
	for _, p := range paths {
		require.NotEmpty(t, p)
		require.Equal(t, "n0", p[0])
		for i := 1; i < len(p); i++ {
			require.True(t, validEdges[p[i-1]][p[i]], "edge %s -> %s must exist", p[i-1], p[i])
		}
	}
}

func TestComputeDataFlowPaths_BoundedStepsOnDeadEnds(t *testing.T) {
	// A wide graph where every leaf only points to itself has no sink, so the
	// uncapped DFS would walk every node and never emit a path. The visit
	// budget must stop it with bounded work.
	total := maxDataFlowVisitBudget + 1
	nodes := make([]*NodeInfo, total)
	for i := range nodes {
		nodes[i] = &NodeInfo{NodeID: fmt.Sprintf("n%d", i)}
	}
	nodes[0].IsEntryNode = true

	edges := make([]*EdgeInfo, 0, total*2-1)
	for i := 0; i < total; i++ {
		self := fmt.Sprintf("n%d", i)
		edges = append(edges, &EdgeInfo{EdgeID: self + "-self", FromNodeID: self, ToNodeID: self})
		if i > 0 {
			edges = append(edges, &EdgeInfo{EdgeID: self + "-from-entry", FromNodeID: "n0", ToNodeID: self})
		}
	}

	paths := computeDataFlowPaths(nodes, edges)
	assert.Empty(t, paths)
}

func TestComputeDataFlowPaths_TruncatesLongChain(t *testing.T) {
	// A chain longer than the length cap must return one bounded path instead
	// of either exhausting the stack or silently dropping the flow.
	total := maxDataFlowPathLength + 100
	nodes := make([]*NodeInfo, total)
	for i := range nodes {
		nodes[i] = &NodeInfo{NodeID: fmt.Sprintf("n%d", i)}
	}
	nodes[0].IsEntryNode = true

	edges := make([]*EdgeInfo, 0, total-1)
	for i := 0; i+1 < total; i++ {
		edges = append(edges, &EdgeInfo{
			EdgeID:     fmt.Sprintf("e%d", i),
			FromNodeID: fmt.Sprintf("n%d", i),
			ToNodeID:   fmt.Sprintf("n%d", i+1),
		})
	}

	paths := computeDataFlowPaths(nodes, edges)
	require.Len(t, paths, 1)
	require.Len(t, paths[0], maxDataFlowPathLength)
	assert.Equal(t, "n0", paths[0][0])
	assert.Equal(t, fmt.Sprintf("n%d", maxDataFlowPathLength-1), paths[0][maxDataFlowPathLength-1])
}

func chainNodes(n int) []*NodeInfo {
	nodes := make([]*NodeInfo, n)
	for i := range nodes {
		nodes[i] = &NodeInfo{NodeID: fmt.Sprintf("n%d", i)}
	}
	return nodes
}
