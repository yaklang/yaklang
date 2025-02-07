package rewriter

import (
	"slices"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	utils2 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	"github.com/yaklang/yaklang/common/utils"
)

type rewriterFunc func(statementManager *RewriteManager, node *core.Node) error

func IfRewriter(manager *RewriteManager, ifNode *core.Node) error {
	core.DumpNodesToDotExp(manager.RootNode)
	err := CalcEnd(manager.DominatorMap, ifNode)
	if err != nil {
		return err
	}
	trueNode := ifNode.TrueNode()
	falseNode := ifNode.FalseNode()
	//ifNode.RemoveAllNext()
	if trueNode == falseNode {
		trueNode = nil
		trueNode = nil
	}
	domNodes := utils2.NodeFilter(ifNode.Next, func(node *core.Node) bool {
		return slices.Contains(manager.DominatorMap[ifNode], node)
	})
	for _, node := range domNodes {
		ifNode.RemoveNext(node)
	}
	ifStatement := statements.NewIfStatement(nil, nil, nil)
	originNodeStatement := ifNode.Statement

	ifStatementNode := manager.NewNode(ifStatement)
	ifNode.Replace(ifStatementNode)
	checkIsEndNode := func(node1, node2 *core.Node) bool {
		if node1 == nil || node2 == nil {
			return false
		}
		endNodes := []*core.Node{}
		core.WalkGraph[*core.Node](node1, func(node *core.Node) ([]*core.Node, error) {
			var next []*core.Node
			for _, n := range node.Next {
				if slices.Contains(manager.DominatorMap[node], n) {
					next = append(next, n)
				}
			}
			if len(next) == 0 {
				endNodes = append(endNodes, node)
			}
			return next, nil
		})
		endNodes = NodeDeduplication(endNodes)
		hasNext := false
		for _, node := range endNodes {
			for _, n := range node.Next {
				hasNext = true
				if n != node2 {
					return false
				}
			}
		}
		if hasNext {
			return true
		}
		return false
	}
	if checkIsEndNode(trueNode, falseNode) {
		falseNode = nil
	}
	if checkIsEndNode(falseNode, trueNode) {
		trueNode = nil
	}
	endNodes := []*core.Node{}
	getBody := func(bodyStartNode *core.Node) ([](*core.Node), error) {
		sts := []*core.Node{}
		if !slices.Contains(manager.DominatorMap[ifNode], bodyStartNode) {
			return sts, nil
		}
		err := core.WalkGraph[*core.Node](bodyStartNode, func(node *core.Node) ([]*core.Node, error) {
			err := manager.CheckVisitedNode(node)
			if err != nil {
				return nil, err
			}
			sts = append(sts, node)
			var next []*core.Node
			for _, n := range node.Next {
				if slices.Contains(manager.DominatorMap[node], n) {
					next = append(next, n)
				} else {
					endNodes = append(endNodes, n)
				}
			}
			return next, nil
		})
		if err != nil {
			return nil, err
		}
		return sts, nil
	}
	condition := originNodeStatement.(*statements.ConditionStatement).Condition
	ifStatement.Condition = condition
	ifBodyNodes := []*core.Node{}
	if trueNode != nil {
		ifBody, err := getBody(trueNode)
		if err != nil {
			return err
		}
		ifStatement.IfBody = core.NodesToStatements(ifBody)
		ifBodyNodes = append(ifBodyNodes, ifBody...)
	}
	if falseNode != nil {
		elseBody, err := getBody(falseNode)
		if err != nil {
			return err
		}
		ifStatement.ElseBody = core.NodesToStatements(elseBody)
		ifBodyNodes = append(ifBodyNodes, elseBody...)
	}
	endNodes = utils2.NodeFilter(endNodes, func(node *core.Node) bool {
		if slices.Contains(ifBodyNodes, node) {
			return false
		}
		return !IsEndNode(node)
	})
	for _, node := range NodeDeduplication(endNodes) {
		ifStatementNode.AddNext(node)
	}

	return nil
}

func CalcEnd1(domTree map[*core.Node][]*core.Node,ifNode *core.Node) error {
	trueNode := ifNode.TrueNode()
	falseNode := ifNode.FalseNode()
	
	// 获取从trueNode出发可以到达的所有节点和路径
	trueNodeSet := utils.NewSet[*core.Node]()
	trueNodePaths := make(map[*core.Node][][]*core.Node)
	trueNodePaths[trueNode] = [][]*core.Node{{trueNode,trueNode}}
	core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
		next := []*core.Node{}
		for _, n := range node.Next {
			if n != ifNode {
				next = append(next, n)
				// 记录到达n的所有路径
				if len(trueNodePaths[node]) == 0 {
					trueNodePaths[n] = append(trueNodePaths[n], []*core.Node{node, n})
				} else {
					for _, path := range trueNodePaths[node] {
						newPath := append(append([]*core.Node{}, path...), n)
						trueNodePaths[n] = append(trueNodePaths[n], newPath)
					}
				}
			}
		}
		trueNodeSet.Add(node)
		return next, nil
	})

	// 获取从falseNode出发可以到达的所有节点和路径
	falseNodeSet := utils.NewSet[*core.Node]()
	falseNodePaths := make(map[*core.Node][][]*core.Node)
	falseNodePaths[falseNode] = [][]*core.Node{{falseNode,falseNode}}
	core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
		next := []*core.Node{}
		for _, n := range node.Next {
			if n != ifNode {
				next = append(next, n)
				// 记录到达n的所有路径
				if len(falseNodePaths[node]) == 0 {
					falseNodePaths[n] = append(falseNodePaths[n], []*core.Node{node, n})
				} else {
					for _, path := range falseNodePaths[node] {
						newPath := append(append([]*core.Node{}, path...), n)
						falseNodePaths[n] = append(falseNodePaths[n], newPath)
					}
				}
			}
		}
		falseNodeSet.Add(node)
		return next, nil
	})

	// 找到所有true和false分支路径都经过的节点中最近的一个
	var mergeNode *core.Node
	minDepth := -1
	for node := range trueNodePaths {
		if falseNodePaths[node] != nil {
			depth := len(trueNodePaths[node][0])
			if minDepth == -1 || depth < minDepth {
				minDepth = depth
				mergeNode = node
			}
		}
	}
ifNode.MergeNode = mergeNode
	return nil
}
func __CalcEnd(domTree map[*core.Node][]*core.Node, ifNode *core.Node) error {
	ifNode.MergeNode = nil
	trueNode := ifNode.TrueNode()
	falseNode := ifNode.FalseNode()

	// 获取从trueNode和falseNode出发可以到达的所有节点
	trueNodeSet := utils.NewSet[*core.Node]()
	falseNodeSet := utils.NewSet[*core.Node]()

	// 遍历true分支可达的所有节点
	if trueNode != nil {
		err := core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
			if node == ifNode {
				return nil, nil
			}
			trueNodeSet.Add(node)
			return node.Next, nil
		})
		if err != nil {
			return err
		}
	}

	// 遍历false分支可达的所有节点
	if falseNode != nil {
		err := core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
			if node == ifNode {
				return nil, nil
			}
			falseNodeSet.Add(node)
			return node.Next, nil
		})
		if err != nil {
			return err
		}
	}

	// 找出两个分支都经过的节点中,最先被访问到的那个节点作为汇聚点
	var mergeNode *core.Node
	minDepth := -1

	// 遍历true分支节点
	for _, node := range trueNodeSet.List() {
		if falseNodeSet.Has(node) {
			// 计算该节点到ifNode的最短路径长度
			depth := 0
			current := node
			for current != nil && current != ifNode {
				depth++
				if len(current.Source) > 0 {
					current = current.Source[0]
				} else {
					current = nil
				}
			}
			
			// 更新最短路径的汇聚点
			if minDepth == -1 || depth < minDepth {
				minDepth = depth
				mergeNode = node
			}
		}
	}

	ifNode.MergeNode = mergeNode
	return nil
}
func CalcEnd(domTree map[*core.Node][]*core.Node, ifNode *core.Node) error {
	ifNode.MergeNode = nil
	trueNode := ifNode.TrueNode()
	falseNode := ifNode.FalseNode()

	domTree = GenerateDominatorTree(ifNode)
	doms := domTree[ifNode]
	switch len(doms) {
	case 1:
		ok1 := false
		if trueNode != nil {
			err := core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
				if node == ifNode {
					return nil, nil
				}
				if node == doms[0] {
					ok1 = true
					return nil, nil
				}
				return node.Next, nil
			})
			if err != nil {
				return err
			}
		}
		ok2 := false
		if falseNode != nil {
			err := core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
				if node == ifNode {
					return nil, nil
				}
				if node == doms[0] {
					ok2 = true
					return nil, nil
				}
				return node.Next, nil
			})
			if err != nil {
				return err
			}
		}

		if ok1 && ok2 {
			ifNode.MergeNode = doms[0]
		}
	case 2:
		for _, dom := range doms {
			ok1 := false
			err := core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
				if node == ifNode {
					return nil, nil
				}
				if node == dom {
					ok1 = true
					return nil, nil
				}
				return node.Next, nil
			})
			if err != nil {
				return err
			}
			ok2 := false
			err = core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
				if node == ifNode {
					return nil, nil
				}
				if node == dom {
					ok2 = true
					return nil, nil
				}
				return node.Next, nil
			})
			if err != nil {
				return err
			}
			if ok1 && ok2 {
				ifNode.MergeNode = dom
				break
			}
		}
	case 3:
		ifNode.MergeNode = utils2.NodeFilter(doms, func(node *core.Node) bool {
			return node != trueNode && node != falseNode
		})[0]
	}
	return nil
}
