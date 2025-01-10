package dot

import "github.com/yaklang/yaklang/common/utils/graph"

func GraphPathPrev(g *Graph, nodeId int) [][]string {
	return graph.GraphPathWithValue(nodeId,
		func(i int) []int {
			node := g.GetNodeByID(i)
			return node.Prevs()
		},
		func(i int) string { // get value
			return NodeName(i)
		},
	)
}

func GraphPathNext(g *Graph, nodeId int) [][]string {
	return graph.GraphPathWithValue(nodeId,
		func(i int) []int {
			node := g.GetNodeByID(i)
			return node.Nexts()
		},
		func(i int) string { // get value
			return NodeName(i)
		},
	)
}
