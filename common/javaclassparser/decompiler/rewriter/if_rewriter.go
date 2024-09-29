package rewriter

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	utils2 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	"github.com/yaklang/yaklang/common/utils"
)

type rewriterFunc func(statementManager *StatementManager, node *core.Node) error

func RewriteIf(statementManager *StatementManager, node *core.Node) error {
	ifNode := node
	if _, ok := ifNode.Statement.(*core.ConditionStatement); !ok {
		return nil
	}
	if len(ifNode.Next) != 2 { // if statement must have two next node
		return nil
	}
	ifBodyStartNode := ifNode.Next[1]   // second next node is if body start node
	elseBodyStartNode := ifNode.Next[0] // first next node is else body start node

	// search merge node, if founded successful, conform that this is an if statement
	ifBodyManager := NewStatementManager(ifBodyStartNode, statementManager)
	ifNodeRecord := statementManager.RewriterContext.ifChildSet[ifNode.Id]
	var mergeNode *core.Node
	elseBodyManager := NewStatementManager(elseBodyStartNode, statementManager)
	elseNodeRecord := utils.NewSet[*core.Node]()
	elseBodyManager.ScanStatement(func(node *core.Node) (error, bool) {
		if ifNodeRecord.Has(node) {
			mergeNode = node
			return nil, false
		}
		elseNodeRecord.Add(node)
		return nil, true
	})
	if mergeNode == nil {
		return nil
	}
	// parse if body and else body
	checkStartAndEnd := func(node *core.Node) bool {
		return node == mergeNode || node == ifNode
	}
	var ifBodyNodes, elseBodyNodes []*core.Node
	if ifBodyStartNode != mergeNode {
		if ifBodyStartNode.Id == 28 {
			print()
		}
		linkNodeFunc := utils2.CutNode(node, ifBodyStartNode)
		err := ifBodyManager.Rewrite()
		linkNodeFunc()
		if err != nil {
			return nil
		}
		ifBody, err := ifBodyManager.ToStatements(func(node *core.Node) bool {
			if checkStartAndEnd(node) {
				return false
			}
			return true
		})
		if err != nil {
			return nil
		}
		ifBodyNodes = ifBody
	}
	if elseBodyStartNode != mergeNode {
		linkNodeFunc := utils2.CutNode(node, elseBodyStartNode)
		err := elseBodyManager.Rewrite()
		linkNodeFunc()
		if err != nil {
			return nil
		}
		elseBody, err := elseBodyManager.ToStatements(func(node *core.Node) bool {
			if checkStartAndEnd(node) {
				return false
			}
			return true
		})
		if err != nil {
			return nil
		}
		elseBodyNodes = elseBody
	}
	// if body must be terminal with goto statement
	lastIfBodyNode := utils.GetLastElement(ifBodyNodes)
	lastElseBodyNode := utils.GetLastElement(elseBodyNodes)
	if lastIfBodyNode != nil { // delete goto statement
		if _, ok := lastIfBodyNode.Statement.(*core.GOTOStatement); !ok {
			return nil
		}
		ifBodyNodes = ifBodyNodes[:len(ifBodyNodes)-1]
		statementManager.DeleteStatementById(lastIfBodyNode.Id)
	}

	// entire if statement is an entity, need to find if statement source and target
	ifBodyStatements := utils2.NodesToStatements(ifBodyNodes)
	elseBodyStatements := utils2.NodesToStatements(elseBodyNodes)
	allStatementNodeSet := utils.NewSet[*core.Node]()
	allStatementNodeSet.Add(ifNode)
	allStatementNodeSet.Add(mergeNode)
	for _, n := range ifBodyNodes {
		allStatementNodeSet.Add(n)
	}
	for _, n := range elseBodyNodes {
		allStatementNodeSet.Add(n)
	}
	// find all source and target
	allSources := [][2]*core.Node{}
	allNext := [][2]*core.Node{}
	for _, n := range allStatementNodeSet.List() {
		for _, n2 := range n.Source {
			if !allStatementNodeSet.Has(n2) {
				allSources = append(allSources, [2]*core.Node{n2, n})
			}
		}
		for _, n2 := range n.Next {
			if !allStatementNodeSet.Has(n2) {
				allNext = append(allNext, [2]*core.Node{n, n2})
			}
		}
	}
	ifStatement := core.NewIfStatement(ifNode.Statement.(*core.ConditionStatement).Condition, ifBodyStatements, elseBodyStatements)
	ifStatementNode := core.NewNode(ifStatement)
	//statementManager.DeleteStatementById(ifNode.Id)
	ifStatementNode.Id = ifNode.Id
	//ifStatementNode.Source = node.Source
	*node = *ifStatementNode
	ifStatementNode = node

	for _, source := range allSources {
		ifStatementNode.Source = append(ifStatementNode.Source, source[0])
		source[0].Next = funk.Filter(source[0].Next, func(item *core.Node) bool {
			return item != source[1]
		}).([]*core.Node)
		source[0].Next = append(source[0].Next, ifStatementNode)
	}
	for _, next := range allNext {
		ifStatementNode.Next = append(ifStatementNode.Next, next[1])
		next[1].Source = funk.Filter(next[1].Source, func(item *core.Node) bool {
			return item != next[0]
		}).([]*core.Node)
		next[1].Source = append(next[1].Source, ifStatementNode)
	}
	//for _, n := range ifNode.Source {
	//	for i, nextNode := range n.Next {
	//		if nextNode.Id == ifNode.Id {
	//			n.Next[i] = ifStatementNode
	//		}
	//	}
	//}

	if lastIfBodyNode != nil && lastIfBodyNode.Next != nil && lastIfBodyNode.Next[0] == ifStatementNode {
		if _, ok := lastIfBodyNode.Statement.(*core.GOTOStatement); !ok {
			gotoStat := core.NewGOTOStatement()
			gotoStat.ToStatement = ifStatementNode.Id
			newNode := core.NewNode(gotoStat)
			newNode.Id = statementManager.GetNewNodeId()
			lastIfBodyNode.Next = []*core.Node{newNode}
			ifStatement.IfBody = append(ifStatement.IfBody, newNode.Statement)
		}
	}
	if lastElseBodyNode != nil && lastElseBodyNode.Next != nil && lastElseBodyNode.Next[0] == ifStatementNode {
		if _, ok := lastElseBodyNode.Statement.(*core.GOTOStatement); !ok {
			gotoStat := core.NewGOTOStatement()
			gotoStat.ToStatement = ifStatementNode.Id
			newNode := core.NewNode(gotoStat)
			newNode.Id = statementManager.GetNewNodeId()
			lastElseBodyNode.Next = []*core.Node{newNode}
			ifStatement.ElseBody = append(ifStatement.ElseBody, newNode.Statement)
		}
	}
	//mergeNode.Source = append(mergeNode.Source, ifStatementNode)

	return nil
}
