package graph_test

import (
	"context"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/graph"
)

// Create a test graph structure
type testGraph struct {
	edges map[string][]string
}

// Test GraphPathWithTarget
func (g *testGraph) getEdge(n string) []string {
	if edges, ok := g.edges[n]; ok {
		return edges
	}
	return nil
}

func (g *testGraph) AddEdge(node string, to ...string) {
	if g.edges == nil {
		g.edges = make(map[string][]string)
	}
	g.edges[node] = append(g.edges[node], to...)
}

func TestGraph_Builder(t *testing.T) {
	t.Run("test dynamic keys with DFSBuilder and dot", func(t *testing.T) {
		// Create a test graph instance
		g := &testGraph{}

		/*
			n1 -> n2
			n1 -> n3
			n1 -> n4

			n2 -> n5
			n3 -> n5

			n5 -> n6
			n4 -> n6
		*/
		g.AddEdge("n1", "n2", "n3", "n4")
		g.AddEdge("n2", "n5")
		g.AddEdge("n3", "n5")
		g.AddEdge("n4", "n6")
		g.AddEdge("n5", "n6")

		edgeTypeUUID := uuid.NewString()

		type edge struct {
			from     string
			to       string
			edgeType string
		}

		saveNode := make([]string, 0)
		saveEdge := make([]edge, 0)

		// Use GraphBuilder to build the graph
		builder := graph.NewDFSGraphBuilder[string, string](
			func(node string) (string, error) {
				saveNode = append(saveNode, node)
				return node, nil
			},
			func(node string) []*graph.Neighbor[string] {
				edges := g.getEdge(node)
				if edges == nil {
					return nil
				}

				neighbors := make([]*graph.Neighbor[string], 0, len(edges))
				for _, edge := range edges {
					neighbors = append(neighbors, &graph.Neighbor[string]{
						Node:     edge,
						EdgeType: edgeTypeUUID,
					})
				}
				return neighbors
			},
			func(fromKey, toKey string, edgeType string, extraMsg map[string]any) {
				saveEdge = append(saveEdge, edge{
					from:     fromKey,
					to:       toKey,
					edgeType: edgeType,
				})
			},
		)

		// Build the graph starting from n1
		builder.BuildGraph("n1")

		// check node
		require.Equal(t, len(saveNode), 6, "Should have 6 nodes")
		require.Contains(t, saveNode, "n1")
		require.Contains(t, saveNode, "n2")
		require.Contains(t, saveNode, "n3")
		require.Contains(t, saveNode, "n4")
		require.Contains(t, saveNode, "n5")
		require.Contains(t, saveNode, "n6")
		// check edge
		require.Equal(t, len(saveEdge), 7, "Should have 6 edges")
		require.Contains(t, saveEdge, edge{from: "n1", to: "n2", edgeType: edgeTypeUUID})
		require.Contains(t, saveEdge, edge{from: "n1", to: "n3", edgeType: edgeTypeUUID})
		require.Contains(t, saveEdge, edge{from: "n1", to: "n4", edgeType: edgeTypeUUID})
		require.Contains(t, saveEdge, edge{from: "n2", to: "n5", edgeType: edgeTypeUUID})
		require.Contains(t, saveEdge, edge{from: "n3", to: "n5", edgeType: edgeTypeUUID})
		require.Contains(t, saveEdge, edge{from: "n4", to: "n6", edgeType: edgeTypeUUID})
		require.Contains(t, saveEdge, edge{from: "n5", to: "n6", edgeType: edgeTypeUUID})

		// Find all paths from n1 to n3
		paths := graph.GraphPathWithTarget[string](
			context.Background(),
			"n1",
			"n3",
			g.getEdge,
		)
		spew.Dump(paths)
		// There should be 1 paths from n1 to n3
		require.Equal(t, 1, len(paths), "Should find 1 path from n1 to n3")
		require.Contains(t, paths, []string{"n1", "n3"})

		// // Also verify using dot's path finding
		paths = graph.GraphPathWithValue("n1", g.getEdge, func(node string) string {
			return node
		})
		spew.Dump(paths)
		require.Equal(t, 3, len(paths), "Should find 3 paths from n1 to n3")
		require.Contains(t, paths, []string{"n1", "n2", "n5", "n6"})
		require.Contains(t, paths, []string{"n1", "n3", "n5", "n6"})
		require.Contains(t, paths, []string{"n1", "n4", "n6"})

	})

}

func TestDFSPathTarget(t *testing.T) {
	// Initialize the test g
	g := &testGraph{}

	t.Run("test graph path with target", func(t *testing.T) {
		// Add edges to the graph
		// The graph structure looks like:
		//
		//         n1
		//       / | \
		//      /  |  \
		//     v   v   v
		//    n2   n3   n4
		//     |   |    |
		//     v   v    |
		//      \  |    |
		//       v v    |
		//        n5    |
		//        |     |
		//        v     v
		//        n6 <---
		//
		g.AddEdge("n1", "n2", "n3", "n4")
		g.AddEdge("n2", "n5")
		g.AddEdge("n3", "n5")
		g.AddEdge("n4", "n6")
		g.AddEdge("n5", "n6")

		paths := graph.GraphPathWithTarget[string](
			context.Background(),
			"n1",
			"n6",
			g.getEdge,
		)
		spew.Dump(paths)
		require.Equal(t, len(paths), 3, "Should find 3 paths from n1 to n6")
		require.Contains(t, paths, []string{"n1", "n2", "n5", "n6"})
		require.Contains(t, paths, []string{"n1", "n3", "n5", "n6"})
		require.Contains(t, paths, []string{"n1", "n4", "n6"})

		paths = graph.GraphPathWithTarget[string](
			context.Background(),
			"n1",
			"n4",
			g.getEdge,
		)
		spew.Dump(paths)
		require.Equal(t, len(paths), 1, "Should find 1 path from n1 to n4")
		require.Contains(t, paths, []string{"n1", "n4"})
	})

	t.Run("test graph path with complex edges", func(t *testing.T) {
		// Reset the graph for this test
		g := &testGraph{}

		// Add edges to create a more complex graph structure
		//
		//         n1
		//       /    \
		//      /      \
		//     v        v
		//    n2        n4
		//     |       /   \
		//     v      v     v
		//    n3     n5 --> n6
		//                  /
		//                 /
		//                v
		//                n7
		//
		g.AddEdge("n1", "n2", "n4")
		g.AddEdge("n2", "n3")
		g.AddEdge("n4", "n5", "n6")
		g.AddEdge("n5", "n6")
		g.AddEdge("n6", "n7")

		// Test path from n1 to n7
		paths := graph.GraphPathWithTarget[string](
			context.Background(),
			"n1",
			"n7",
			g.getEdge,
		)

		spew.Dump(paths)
		// We should find multiple paths from n1 to n7
		require.NotEmpty(t, paths, "Should find paths from n1 to n7")

		// Check for some expected paths
		require.Contains(t, paths, []string{"n1", "n4", "n6", "n7"})
		require.Contains(t, paths, []string{"n1", "n4", "n5", "n6", "n7"})

		// Test a different target
		paths = graph.GraphPathWithTarget[string](
			context.Background(),
			"n1",
			"n3",
			g.getEdge,
		)

		spew.Dump(paths)
		require.NotEmpty(t, paths, "Should find paths from n1 to n6")
		require.Contains(t, paths, []string{"n1", "n2", "n3"})
	})

}

func TestDFSPathContext(t *testing.T) {
	timeInterval := 100 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeInterval)
	defer cancel()

	start := time.Now()
	paths := graph.GraphPathWithTarget(ctx, "n1", "n6", func(n string) []string {
		log.Infof("call back ")
		time.Sleep(1 * timeInterval) // 3 times of timeInterval
		return []string{"n2", "n3", "n4"}
	})
	spew.Dump(paths)
	since := time.Since(start)
	spew.Dump(since)
	require.Equal(t, len(paths), 0, "Should not find any paths")
	require.True(t, since > timeInterval, "Should match timeout")
	require.True(t, since < 3*timeInterval, "Should match timeout")
}
