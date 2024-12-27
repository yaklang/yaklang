package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	utils2 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	"github.com/yaklang/yaklang/common/utils"
	"slices"
)

type rewriterFunc func(statementManager *RewriteManager, node *core.Node) error

func IfRewriter(manager *RewriteManager, ifNode *core.Node) error {
	utils2.DumpNodesToDotExp(manager.RootNode)
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

func CalcMergeNode1(ifNode *core.Node) *core.Node {
	trueNode := ifNode.TrueNode()
	falseNode := ifNode.FalseNode()
	trueNodeSet := utils.NewSet[*core.Node]()
	core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
		next := []*core.Node{}
		for _, n := range node.Next {
			if n != ifNode {
				next = append(next, n)
			}
		}
		trueNodeSet.Add(node)
		return next, nil
	})
	var mergeNode *core.Node
	core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
		if mergeNode != nil {
			return nil, nil
		}
		if trueNodeSet.Has(falseNode) {
			mergeNode = node
			return nil, nil
		}
		return node.Next, nil
	})
	return mergeNode
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
		ok2 := false
		err = core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
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
