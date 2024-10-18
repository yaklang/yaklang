package core

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

func GenerateDominatorTree(node *Node) {
	id := 0
	nodeToId := make(map[*Node]int)
	idToNode := make(map[int]*Node)
	semi := make(map[*Node]int)
	dom := make(map[*Node]int)
	ancestor := make(map[*Node]int)
	parent := make(map[*Node]int)
	bucket := make(map[*Node][]*Node)
	var dfs func(node *Node)
	dfs = func(n *Node) {
		nodeToId[n] = id
		idToNode[id] = n
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
		n := idToNode[i]
		if len(n.Source) == 0 {
			continue
		}
		for _, source := range n.Source {
			var nAncestor *Node
			var walkToAncestor func(node *Node) *Node
			walkToAncestor = func(node *Node) *Node {
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
		if dom[node] != semi[node] {
			dom[node] = dom[idToNode[dom[node]]]
		}
	}
	var sb strings.Builder
	sb.WriteString("digraph G {\n")
	for node, dom := range dom {
		n1 := idToNode[dom]
		sb.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", node.Statement.String(&class_context.ClassContext{}), n1.Statement.String(&class_context.ClassContext{})))
	}
	sb.WriteString("}\n")
	println(sb.String())
}
