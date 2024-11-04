package core

import (
	"github.com/yaklang/yaklang/common/utils"
)

func GenerateDominatorTree(node *OpCode) map[*OpCode][]*OpCode {
	nodes := []*OpCode{}
	sourceMap := map[*OpCode][]*OpCode{}
	err := WalkGraph[*OpCode](node, func(node *OpCode) ([]*OpCode, error) {
		nodes = append(nodes, node)
		for _, p := range node.Source {
			sourceMap[p] = append(sourceMap[p], node)
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
	dMap[node] = utils.NewSet[*OpCode]()
	dMap[node].Add(node)
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
				if dMap[p] == nil {
					continue
				}
				netSet = netSet.And(dMap[p])
			}
			netSet.Add(nodes[i])
			if netSet.Diff(dMap[nodes[i]]).Len() != 0 {
				dMap[nodes[i]] = netSet
				flag = true
			}
		}
	}
	dominatorMap := map[*OpCode][]*OpCode{}
	for node, dom := range dMap {
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
	}
	return dominatorMap
}
