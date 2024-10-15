package core

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"strings"
)

type Graph struct {
	edges map[*Node][]*Node
	nodes []*Node
}

func NewGraph() *Graph {
	return &Graph{edges: make(map[*Node][]*Node)}
}

func (g *Graph) AddEdge(from, to *Node) {
	g.edges[from] = append(g.edges[from], to)
	if !contains(g.nodes, from) {
		g.nodes = append(g.nodes, from)
	}
	if !contains(g.nodes, to) {
		g.nodes = append(g.nodes, to)
	}
}

func contains(slice []*Node, item *Node) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

func (g *Graph) DominatorTree(start *Node) map[*Node]*Node {
	// Create a mapping from node IDs to indices
	nodeIndex := make(map[*Node]int)
	indexNode := make(map[int]*Node)
	for i, node := range g.nodes {
		nodeIndex[node] = i
		indexNode[i] = node
	}

	n := len(g.nodes)
	semi := make([]int, n)
	idom := make([]int, n)
	parent := make([]int, n)
	vertex := make([]int, n)
	ancestor := make([]int, n)
	label := make([]int, n)
	bucket := make([][]int, n)
	for i := 0; i < n; i++ {
		semi[i] = -1
		idom[i] = -1
		parent[i] = -1
		ancestor[i] = -1
		label[i] = i
	}

	dfsNum := 0
	var dfs func(int)
	dfs = func(v int) {
		semi[v] = dfsNum
		vertex[dfsNum] = v
		dfsNum++
		for _, w := range g.edges[indexNode[v]] {
			wIndex := nodeIndex[w]
			if semi[wIndex] == -1 {
				parent[wIndex] = v
				dfs(wIndex)
			}
		}
	}
	dfs(nodeIndex[start])

	for i := dfsNum - 1; i > 0; i-- {
		w := vertex[i]
		for _, v := range g.edges[indexNode[w]] {
			u := eval(nodeIndex[v], ancestor, semi, label)
			if semi[u] < semi[w] {
				semi[w] = semi[u]
			}
		}
		bucket[vertex[semi[w]]] = append(bucket[vertex[semi[w]]], w)
		link(parent[w], w, ancestor)
		for _, v := range bucket[parent[w]] {
			u := eval(v, ancestor, semi, label)
			if semi[u] < semi[v] {
				idom[v] = u
			} else {
				idom[v] = parent[w]
			}
		}
		bucket[parent[w]] = nil
	}

	for i := 1; i < dfsNum; i++ {
		w := vertex[i]
		if idom[w] != vertex[semi[w]] {
			idom[w] = idom[idom[w]]
		}
	}
	idom[nodeIndex[start]] = -1

	dominatorTree := make(map[*Node]*Node)
	for i, v := range idom {
		if v != -1 {
			dominatorTree[indexNode[i]] = indexNode[v]
		}
	}
	return dominatorTree
}

func eval(v int, ancestor, semi, label []int) int {
	if ancestor[v] == -1 {
		return v
	}
	compress(v, ancestor, label)
	return label[v]
}

func compress(v int, ancestor, label []int) {
	if ancestor[ancestor[v]] != -1 {
		compress(ancestor[v], ancestor, label)
		if label[ancestor[v]] < label[v] {
			label[v] = label[ancestor[v]]
		}
		ancestor[v] = ancestor[ancestor[v]]
	}
}

func link(v, w int, ancestor []int) {
	ancestor[w] = v
}

func GenerateDominatorTree(node *Node) {
	g := NewGraph()
	WalkGraph(node, func(n *Node) ([]*Node, error) {
		for _, next := range n.Next {
			g.AddEdge(n, next)
		}
		return n.Next, nil
	})
	dominatorTree := g.DominatorTree(node)
	var sb strings.Builder
	sb.WriteString("digraph G {\n")
	for node, dom := range dominatorTree {
		sb.WriteString(fmt.Sprintf("  \"%d\" -> \"%d\";\n", dom.Statement.String(&class_context.FunctionContext{}), node.Statement.String(&class_context.FunctionContext{})))
	}
	sb.WriteString("}\n")

}
