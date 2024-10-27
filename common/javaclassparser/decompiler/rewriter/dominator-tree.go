package rewriter

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/utils"
	"sort"
)

type Graph struct {
	Nodes int
	Edges [][]int
}

func NewGraph(nodes int) *Graph {
	return &Graph{
		Nodes: nodes,
		Edges: make([][]int, nodes),
	}
}

func (g *Graph) AddEdge(from, to int) {
	g.Edges[from] = append(g.Edges[from], to)
}

func (g *Graph) Reverse() *Graph {
	rg := NewGraph(g.Nodes)
	for u, neighbors := range g.Edges {
		for _, v := range neighbors {
			rg.AddEdge(v, u)
		}
	}
	return rg
}

func DFS(g *Graph, u int, visited []bool, order *[]int) {
	visited[u] = true
	for _, v := range g.Edges[u] {
		if !visited[v] {
			DFS(g, v, visited, order)
		}
	}
	*order = append(*order, u)
}

func LengauerTarjan(g *Graph, start int) []int {
	n := g.Nodes
	rg := g.Reverse()

	// Step 1: Perform DFS and calculate postorder
	order := []int{}
	visited := make([]bool, n)
	DFS(g, start, visited, &order)
	sort.Slice(order, func(i, j int) bool { return order[i] > order[j] })

	// Step 2: Initialize variables
	ancestor := make([]int, n)
	label := make([]int, n)
	semi := make([]int, n)
	parent := make([]int, n)
	dom := make([]int, n)
	bucket := make([][]int, n)

	for i := range semi {
		semi[i] = i
		label[i] = i
		ancestor[i] = -1
		dom[i] = -1
	}

	// Step 3: Process nodes in reverse postorder
	for _, w := range order {
		for _, v := range rg.Edges[w] {
			u := eval(v, ancestor, label, semi)
			if semi[u] < semi[w] {
				semi[w] = semi[u]
			}
		}
		bucket[semi[w]] = append(bucket[semi[w]], w)
		link(parent[w], w, ancestor)
		for _, v := range bucket[parent[w]] {
			u := eval(v, ancestor, label, semi)
			if semi[u] < semi[v] {
				dom[v] = u
			} else {
				dom[v] = parent[w]
			}
		}
		bucket[parent[w]] = nil
	}

	// Step 4: Finalize dominators
	for i := 1; i < len(order); i++ {
		w := order[i]
		if dom[w] != semi[w] {
			dom[w] = dom[dom[w]]
		}
	}

	dom[start] = start
	return dom
}

func link(v, w int, ancestor []int) {
	ancestor[w] = v
}

func eval(v int, ancestor, label, semi []int) int {
	if ancestor[v] == -1 {
		return v
	}
	compress(v, ancestor, label, semi)
	return label[v]
}

func compress(v int, ancestor, label, semi []int) {
	if ancestor[ancestor[v]] != -1 {
		compress(ancestor[v], ancestor, label, semi)
		if semi[label[ancestor[v]]] < semi[label[v]] {
			label[v] = label[ancestor[v]]
		}
		ancestor[v] = ancestor[ancestor[v]]
	}
}

func _GenerateDominatorTree(node *core.Node) {
	g := NewGraph(7)
	g.AddEdge(0, 1)
	g.AddEdge(1, 2)
	g.AddEdge(1, 3)
	g.AddEdge(2, 4)
	g.AddEdge(3, 4)
	g.AddEdge(4, 5)
	g.AddEdge(5, 6)

	dom := LengauerTarjan(g, 0)
	fmt.Println("Dominators:", dom)
	//var sb strings.Builder
	//sb.WriteString("digraph G {\n")
	//for node, d := range dom {
	//	n1 := idToNode[dom]
	//	sb.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", node.Statement.String(&class_context.ClassContext{}), n1.Statement.String(&class_context.ClassContext{})))
	//}
	//sb.WriteString("}\n")
	//println(sb.String())
}
func GenerateDominatorTree(node *core.Node) map[*core.Node][]*core.Node {
	id := 0
	nodeToId := make(map[*core.Node]int)
	idToNode := make(map[int]*core.Node)
	nodes := []*core.Node{}
	semi := make(map[*core.Node]int)
	dom := make(map[*core.Node]int)
	ancestor := make(map[*core.Node]int)
	parent := make(map[*core.Node]int)
	bucket := make(map[*core.Node][]*core.Node)
	var dfs func(node *core.Node)
	dfs = func(n *core.Node) {
		nodeToId[n] = id
		idToNode[id] = n
		nodes = append(nodes, n)
		semi[n] = id
		id++
		for _, next := range n.Next {
			if _, ok := nodeToId[next]; !ok {
				parent[next] = nodeToId[n]
				dfs(next)
			}
		}
	}
	dfs(node)

	for i := id - 1; i >= 0; i-- {
		n := nodes[i]
		if len(n.Source) == 0 {
			continue
		}
		for _, source := range n.Source {
			var nAncestor *core.Node
			var walkToAncestor func(node *core.Node) *core.Node
			walkToAncestor = func(node *core.Node) *core.Node {
				currentAncestor := idToNode[ancestor[node]]
				if currentAncestor == node {
					return node
				}
				res := walkToAncestor(currentAncestor)
				if semi[currentAncestor] < semi[node] {
					semi[node] = semi[currentAncestor]
				}
				ancestor[node] = nodeToId[res]
				return res
				//if ancestorNode, ok := ancestor[currentAncestor]; ok {
				//	walkToAncestor(idToNode[ancestorNode])
				//	if semi[currentAncestor] < semi[node] {
				//		semi[node] = semi[currentAncestor]
				//	}
				//	return idToNode[semi[node]]
				//} else {
				//	return node
				//}
			}
			if _, ok := ancestor[source]; !ok {
				nAncestor = source
			} else {
				if _, ok := ancestor[source]; ok {
					nAncestor = walkToAncestor(source)
				}
				//nAncestor = idToNode[semi[node]]
			}
			semi[n] = utils.Min(semi[nAncestor], semi[n])
			ancestor[n] = parent[n]
			bucket[idToNode[semi[n]]] = append(bucket[idToNode[semi[n]]], n)
			for _, v := range bucket[idToNode[parent[n]]] {
				u := walkToAncestor(v)
				if semi[u] == semi[v] {
					dom[v] = nodeToId[u]
				} else {
					dom[v] = semi[u]
				}
			}
			bucket[idToNode[parent[source]]] = nil
		}
	}
	//for i in range(1, len(vertex)):
	//w = vertex[i]
	//if dom[w] != vertex[semi[w]]:
	//dom[w] = dom[dom[w]]
	//
	//dom[start_node] = None
	//for node, i := range semi {
	//	fmt.Printf("%d -> %d\n", node.Id, idToNode[i].Id)
	//}
	for i := 1; i < id-1; i++ {
		node := idToNode[i]
		if v, ok := dom[node]; !ok || v != semi[node] {
			dom[node] = dom[idToNode[dom[node]]]

		}
	}
	//var sb strings.Builder
	//sb.WriteString("digraph G {\n")
	dominatorMap := map[*core.Node][]*core.Node{}
	for node, dom := range dom {
		dominatorMap[idToNode[dom]] = append(dominatorMap[idToNode[dom]], node)
		//n1 := idToNode[dom]
		//sb.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", node.Statement.String(&class_context.ClassContext{}), n1.Statement.String(&class_context.ClassContext{})))
	}
	return dominatorMap
	//sb.WriteString("}\n")
	//println(sb.String())
}
