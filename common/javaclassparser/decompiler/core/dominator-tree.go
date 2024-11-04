package core

import (
	"github.com/yaklang/yaklang/common/utils"
)

func GenerateDominatorTree(rootNode *OpCode) map[*OpCode][]*OpCode {
	nodes := []*OpCode{}
	sourceMap := make(map[*OpCode][]*OpCode)
	err := WalkGraph[*OpCode](rootNode, func(node *OpCode) ([]*OpCode, error) {
		nodes = append(nodes, node)
		for _, n := range node.Target {
			sourceMap[n] = append(sourceMap[n], node)
		}
		return node.Target, nil
	})
	if err != nil {
		return nil
	}
	nodeToId := map[*OpCode]int{}
	for i, n := range nodes {
		nodeToId[n] = i
	}
	dMap := map[*OpCode]*utils.Set[*OpCode]{}
	dMap[rootNode] = utils.NewSet[*OpCode]()
	dMap[rootNode].Add(rootNode)
	//startNode := node
	for i := 1; i < len(nodes); i++ {
		dMap[nodes[i]] = utils.NewSet[*OpCode]()
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
	dominatorMap := map[*OpCode][]*OpCode{}
	for node, dom := range dMap {
		//dominatorMap[idToNode[dom]] = append(dominatorMap[idToNode[dom]], node)
		//n1 := idToNode[dom]
		var idom *OpCode
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
	return dominatorMap

}
