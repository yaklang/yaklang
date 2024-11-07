package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/utils"
	"sort"
)

func GenerateDominatorTree(rootNode *core.Node) map[*core.Node][]*core.Node {
	nodes := []*core.Node{}
	sourceMap := make(map[*core.Node][]*core.Node)
	err := core.WalkGraph[*core.Node](rootNode, func(node *core.Node) ([]*core.Node, error) {
		nodes = append(nodes, node)
		for _, n := range node.Next {
			sourceMap[n] = append(sourceMap[n], node)
		}
		return node.Next, nil
	})
	if err != nil {
		return nil
	}
	nodeToId := map[*core.Node]int{}
	for i, n := range nodes {
		nodeToId[n] = i
	}
	dMap := map[*core.Node]*utils.Set[*core.Node]{}
	dMap[rootNode] = utils.NewSet[*core.Node]()
	dMap[rootNode].Add(rootNode)
	//startNode := node
	for i := 1; i < len(nodes); i++ {
		dMap[nodes[i]] = utils.NewSet[*core.Node]()
		dMap[nodes[i]].AddList(nodes)
	}
	flag := true
	for flag {
		flag = false
		for i := 0; i < len(nodes); i++ {
			netSet := dMap[nodes[i]]
			for _, p := range sourceMap[nodes[i]] {
				netSet = netSet.And(dMap[p])
			}
			netSet.Add(nodes[i])
			if netSet.Diff(dMap[nodes[i]]).Len() != 0 {
				dMap[nodes[i]] = netSet
				flag = true
			}
		}
	}

	//var sb strings.Builder
	//sb.WriteString("digraph G {\n")
	dominatorMap := map[*core.Node][]*core.Node{}
	for node, dom := range dMap {
		//dominatorMap[idToNode[dom]] = append(dominatorMap[idToNode[dom]], node)
		//n1 := idToNode[dom]
		var idom *core.Node
		for _, n := range dom.List() {
			if n == node {
				continue
			}
			if idom == nil {
				idom = n
			} else {
				if nodeToId[n] > nodeToId[idom] {
					idom = n
				}
			}
		}
		if idom == nil {
			continue
		}
		dominatorMap[idom] = append(dominatorMap[idom], node)
		//sb.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\";\n", idom.Statement.String(&class_context.ClassContext{}), node.Statement.String(&class_context.ClassContext{})))
	}
	//sb.WriteString("}\n")
	//println(sb.String())
	for _, nodeList := range dominatorMap {
		sort.Slice(nodeList, func(i, j int) bool {
			return nodeToId[nodeList[i]] < nodeToId[nodeList[j]]
		})
	}
	return dominatorMap

}
