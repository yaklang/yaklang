package dot_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/utils/graph"
)

func TestGraph_Builder(t *testing.T) {
	t.Run("test dynamic keys with DFSBuilder and dot", func(t *testing.T) {
		// 创建一个 dot 图实例
		dotGraph := dot.New()
		dotGraph.MakeDirected()
		dotGraph.GraphAttribute("rankdir", "BT")

		var NodeToKey = make(map[string]int)
		// 使用 GraphBuilder 构建图
		builder := graph.NewDFSGraphBuilder[int, string](
			func(node string) (int, error) {
				nodeId := dotGraph.AddNode(node)
				NodeToKey[node] = nodeId
				return nodeId, nil
			},
			func(node string) []*graph.Neighbor[string] {
				switch node {
				case "n1":
					return []*graph.Neighbor[string]{
						{Node: "n2", EdgeType: "depends_on"},
						{Node: "n4", EdgeType: "depends_on"},
					}
				case "n2":
					return []*graph.Neighbor[string]{
						{Node: "n3", EdgeType: "depends_on"},
					}
				case "n4":
					return []*graph.Neighbor[string]{
						{Node: "n3", EdgeType: "depends_on"},
					}
				}
				return nil
			},
			func(fromKey, toKey int, edgeType string, extraMsg map[string]any) {
				switch edgeType {
				case "depends_on":
					dotGraph.AddEdge(fromKey, toKey, "")
				}
			},
		)

		// 构建图
		builder.BuildGraph("n1")

		var buf bytes.Buffer
		dotGraph.GenerateDOT(&buf)
		fmt.Println(buf.String())

		// 验证节点是否存在
		n1 := dotGraph.FindNode("n1")
		require.NotNil(t, n1, "Node n1 should exist in the graph")
		n2 := dotGraph.FindNode("n2")
		require.NotNil(t, n2, "Node n2 should exist in the graph")
		n3 := dotGraph.FindNode("n3")
		require.NotNil(t, n3, "Node n3 should exist in the graph")
		n4 := dotGraph.FindNode("n4")
		require.NotNil(t, n4, "Node n4 should exist in the graph")

		// 验证边是否存在
		require.True(t, dotGraph.HasEdge(n1, n2), "Edge (n1 -> n2) should exist")
		require.True(t, dotGraph.HasEdge(n1, n4), "Edge (n1 -> n4) should exist")
		require.True(t, dotGraph.HasEdge(n4, n3), "Edge (n4 -> n3) should exist")
		require.True(t, dotGraph.HasEdge(n2, n3), "Edge (n4 -> n3) should exist")

		// 验证路径逻辑
		getNodeName := func(node string) string {
			nodeId := NodeToKey[node]
			return dot.NodeName(nodeId)
		}

		pathN1ToN3 := dot.GraphPathPrev(dotGraph, n3.ID())
		require.Equal(t, len(pathN1ToN3), 2, "Path from n1 to n3 should have exactly one path")
		require.Contains(t, pathN1ToN3, []string{getNodeName("n3"), getNodeName("n2"), getNodeName("n1")}, "Path from n3 to n1 should match expected sequence")
		require.Contains(t, pathN1ToN3, []string{getNodeName("n3"), getNodeName("n4"), getNodeName("n1")}, "Path from n3 to n1 should match expected sequence")
	})

	/*
		n1 -> n2
		n1 -> n3
		n1 -> n4

		n2 -> n5
		n3 -> n5

		n5 -> n6
		n4 -> n6
	*/

	t.Run("test dynamic keys with BFSBuilder and dot", func(t *testing.T) {
		// 创建一个 dot 图实例
		dotGraph := dot.New()
		dotGraph.MakeDirected()
		dotGraph.GraphAttribute("rankdir", "BT")

		var NodeToKey = make(map[string]int)
		// 使用 GraphBuilder 构建图
		builder := graph.NewBFSGraphBuilder[int, string](
			func(node string) (int, error) {
				nodeId := dotGraph.AddNode(node)
				NodeToKey[node] = nodeId
				return nodeId, nil
			},
			func(node string) map[string]*graph.Neighbor[string] {
				switch node {
				case "n1":
					return map[string]*graph.Neighbor[string]{
						"n2": {Node: "n2", EdgeType: "depends_on"},
						"n3": {Node: "n3", EdgeType: "depends_on"},
						"n4": {Node: "n4", EdgeType: "depends_on"},
					}
				case "n2":
					return map[string]*graph.Neighbor[string]{
						"n5": {Node: "n5", EdgeType: "depends_on"},
					}
				case "n3":
					return map[string]*graph.Neighbor[string]{
						"n5": {Node: "n5", EdgeType: "depends_on"},
					}
				case "n4":
					return map[string]*graph.Neighbor[string]{
						"n6": {Node: "n6", EdgeType: "depends_on"},
					}
				case "n5":
					return map[string]*graph.Neighbor[string]{
						"n6": {Node: "n6", EdgeType: "depends_on"},
					}
				}
				return nil
			},
			func(node string) map[string]*graph.Neighbor[string] {
				return map[string]*graph.Neighbor[string]{}
			},
			func(fromKey, toKey int, edgeType string, extraMsg map[string]any) {
				switch edgeType {
				case "depends_on":
					dotGraph.AddEdge(fromKey, toKey, "")
				}
			},
		)

		// from n1 to n5
		builder.BuildGraph("n1", "n5")

		var buf bytes.Buffer
		dotGraph.GenerateDOT(&buf)
		fmt.Println(buf.String())

		n1 := dotGraph.FindNode("n1")
		require.NotNil(t, n1, "Node n1 should exist in the graph")
		n2 := dotGraph.FindNode("n2")
		require.NotNil(t, n2, "Node n2 should exist in the graph")
		n3 := dotGraph.FindNode("n3")
		require.NotNil(t, n3, "Node n3 should exist in the graph")
		// n4 := dotGraph.FindNode("n4")
		// require.NotNil(t, n4, "Node n4 should exist in the graph")
		n5 := dotGraph.FindNode("n5")
		require.NotNil(t, n5, "Node n5 should exist in the graph")
		// n6 := dotGraph.FindNode("n6")
		// require.NotNil(t, n6, "Node n6 should exist in the graph")

		require.True(t, dotGraph.HasEdge(n1, n2), "Edge (n1 -> n2) should exist")
		require.True(t, dotGraph.HasEdge(n1, n3), "Edge (n1 -> n3) should exist")
		require.True(t, dotGraph.HasEdge(n2, n5), "Edge (n2 -> n5) should exist")
		require.True(t, dotGraph.HasEdge(n3, n5), "Edge (n3 -> n5) should exist")

		getNodeName := func(node string) string {
			nodeId := NodeToKey[node]
			return dot.NodeName(nodeId)
		}

		pathN1ToN6 := dot.GraphPathPrev(dotGraph, n5.ID())
		require.Equal(t, len(pathN1ToN6), 2, "Path from n1 to n5 should have exactly one path")
		require.Contains(t, pathN1ToN6, []string{getNodeName("n5"), getNodeName("n2"), getNodeName("n1")}, "Path from n5 to n1 should match expected sequence")
		require.Contains(t, pathN1ToN6, []string{getNodeName("n5"), getNodeName("n3"), getNodeName("n1")}, "Path from n5 to n1 should match expected sequence")

		dotGraph = dot.New()
		dotGraph.MakeDirected()
		dotGraph.GraphAttribute("rankdir", "BT")

		// from n1 to n6
		builder.BuildGraph("n1", "n6")

		dotGraph.GenerateDOT(&buf)
		fmt.Println(buf.String())

		n1 = dotGraph.FindNode("n1")
		require.NotNil(t, n1, "Node n1 should exist in the graph")
		n2 = dotGraph.FindNode("n2")
		require.NotNil(t, n2, "Node n2 should exist in the graph")
		n3 = dotGraph.FindNode("n3")
		require.NotNil(t, n3, "Node n3 should exist in the graph")
		n4 := dotGraph.FindNode("n4")
		require.NotNil(t, n4, "Node n4 should exist in the graph")
		n5 = dotGraph.FindNode("n5")
		require.NotNil(t, n5, "Node n5 should exist in the graph")
		n6 := dotGraph.FindNode("n6")
		require.NotNil(t, n6, "Node n6 should exist in the graph")

		require.True(t, dotGraph.HasEdge(n1, n2), "Edge (n1 -> n2) should exist")
		require.True(t, dotGraph.HasEdge(n1, n3), "Edge (n1 -> n3) should exist")
		require.True(t, dotGraph.HasEdge(n2, n5), "Edge (n2 -> n5) should exist")
		require.True(t, dotGraph.HasEdge(n3, n5), "Edge (n3 -> n5) should exist")
		require.True(t, dotGraph.HasEdge(n5, n6), "Edge (n5 -> n6) should exist")
		require.True(t, dotGraph.HasEdge(n1, n4), "Edge (n1 -> n4) should exist")
		require.True(t, dotGraph.HasEdge(n4, n6), "Edge (n4 -> n6) should exist")

		pathN1ToN6 = dot.GraphPathPrev(dotGraph, n6.ID())
		require.Equal(t, len(pathN1ToN6), 3, "Path from n1 to n6 should have exactly one path")
		require.Contains(t, pathN1ToN6, []string{getNodeName("n6"), getNodeName("n5"), getNodeName("n2"), getNodeName("n1")}, "Path from n6 to n1 should match expected sequence")
		require.Contains(t, pathN1ToN6, []string{getNodeName("n6"), getNodeName("n5"), getNodeName("n3"), getNodeName("n1")}, "Path from n6 to n1 should match expected sequence")
		require.Contains(t, pathN1ToN6, []string{getNodeName("n6"), getNodeName("n4"), getNodeName("n1")}, "Path from n6 to n1 should match expected sequence")
	})
}
