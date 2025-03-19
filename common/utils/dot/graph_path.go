package dot

import "github.com/yaklang/yaklang/common/utils/graph"

func GraphPathPrevWithFilter(g *Graph, nodeId int, filter func(edge *Edge) bool) [][]string {
	return graph.GraphPathWithValue(nodeId,
		func(i int) []int {
			ret := []int{}
			node := g.GetNodeByID(i)
			for _, prev := range node.Prevs() {
				edgeId := g.GetEdges(prev, i)
				for _, id := range edgeId {
					e := g.GetEdge(id)
					if e != nil && filter(e) {
						ret = append(ret, prev)
						break
					}
				}
			}
			return ret
		},
		func(i int) string { // get value
			return NodeName(i)
		},
	)
}
func GraphPathPrev(g *Graph, nodeId int) [][]string {
	return GraphPathPrevWithFilter(g, nodeId, func(edge *Edge) bool {
		return true
	})
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
