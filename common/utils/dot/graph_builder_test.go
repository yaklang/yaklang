package dot_test

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/utils/graph"
	"testing"
)

func TestGraph_Builder(t *testing.T) {
	t.Run("test dynamic keys with DFSBuilder and dot", func(t *testing.T) {
		// 创建一个 dot 图实例
		dotGraph := dot.New()
		dotGraph.MakeDirected()
		dotGraph.GraphAttribute("rankdir", "BT")

		// 创建节点的唯一标识->节点
		keyToNode := make(map[int]string)
		// 节点->节点唯一标识
		NodeToKey := make(map[string]int)

		// 使用 GraphBuilder 构建图
		builder := graph.NewDFSGraphBuilder[int, string](
			func(node string) (int, error) {
				if nodeId, ok := NodeToKey[node]; ok {
					return nodeId, nil
				}
				nodeId := dotGraph.AddNode(node)
				NodeToKey[node] = nodeId
				keyToNode[nodeId] = node

				return nodeId, nil
			},
			func(nodeKey int) []*graph.NeighborWithEdgeType[string] {
				if node, ok := keyToNode[nodeKey]; ok {
					switch node {
					case "n1":
						return []*graph.NeighborWithEdgeType[string]{
							{Node: "n2", EdgeType: "depends_on"},
							{Node: "n4", EdgeType: "depends_on"},
						}
					case "n2":
						return []*graph.NeighborWithEdgeType[string]{
							{Node: "n3", EdgeType: "depends_on"},
						}
					case "n4":
						return []*graph.NeighborWithEdgeType[string]{
							{Node: "n3", EdgeType: "depends_on"},
						}
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
}
