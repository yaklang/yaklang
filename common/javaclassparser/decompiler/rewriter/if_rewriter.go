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

	if ifBodyStartNode.Id <= ifNode.Id || elseBodyStartNode.Id <= ifNode.Id {
		return nil
	}
	// search merge node, if founded successful, conform that this is an if statement
	ifBodyManager := NewStatementManager(ifBodyStartNode, statementManager)
	//ifNodeRecord := statementManager.RewriterContext.ifChildSet[ifNode.Id]
	ifNodeRecord := utils.NewSet[*core.Node]()
	ifBodyManager.ScanStatement(func(node *core.Node) (error, bool) {
		ifNodeRecord.Add(node)
		return nil, true
	})
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
	if mergeNode.Id <= ifNode.Id {
		return nil
	}
	// parse if body and else body
	checkStartAndEnd := func(node *core.Node) bool {
		return node == mergeNode
	}
	var ifBodyNodes, elseBodyNodes []*core.Node
	if ifBodyStartNode != mergeNode {
		linkNodeFunc := utils2.CutNode(node, ifBodyStartNode)
		err := ifBodyManager.Rewrite()
		linkNodeFunc()
		if err != nil {
			return err
		}
		ifBody, err := ifBodyManager.ToStatements(func(node *core.Node) bool {
			if checkStartAndEnd(node) {
				return false
			}
			return true
		})
		if err != nil {
			return err
		}
		ifBodyNodes = ifBody
	}
	if elseBodyStartNode != mergeNode {
		linkNodeFunc := utils2.CutNode(node, elseBodyStartNode)
		err := elseBodyManager.Rewrite()
		linkNodeFunc()
		if err != nil {
			return err
		}
		elseBody, err := elseBodyManager.ToStatements(func(node *core.Node) bool {
			if checkStartAndEnd(node) {
				return false
			}
			return true
		})
		if err != nil {
			return err
		}
		elseBodyNodes = elseBody
	}
	// if body must be terminal with goto statement
	lastIfBodyNode := utils.GetLastElement(ifBodyNodes)
	//lastElseBodyNode := utils.GetLastElement(elseBodyNodes)
	if lastIfBodyNode != nil { // delete goto statement

		// because if statement all can jmp code block, so terminal with goto statement is not necessary
		//if _, ok := lastIfBodyNode.Statement.(*core.GOTOStatement); !ok {
		//	return errors.New("if body must be terminal with goto statement")
		//}

		if _, ok := lastIfBodyNode.Statement.(*core.GOTOStatement); ok && lastIfBodyNode.Next[0] == mergeNode { // if body default action is jmp to merge node
			ifBodyNodes = ifBodyNodes[:len(ifBodyNodes)-1]
			statementManager.DeleteStatementById(lastIfBodyNode.Id)
		}
	}

	// entire if statement is an entity, need to find if statement source and target
	ifBodyStatements := utils2.NodesToStatements(ifBodyNodes)
	elseBodyStatements := utils2.NodesToStatements(elseBodyNodes)
	allStatementNodeSet := utils.NewSet[*core.Node]()
	allStatementNodeSet.Add(ifNode)
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
		ifStatementNode.AddSource(source[0])
		source[0].Next = funk.Filter(source[0].Next, func(item *core.Node) bool {
			return item != source[1]
		}).([]*core.Node)
		source[0].AddNext(ifStatementNode)
		//if source[1] != ifNode {
		//	gotoStat := core.NewGOTOStatement()
		//	gotoStat.ToStatement = source[1].Id
		//	newNode := core.NewNode(gotoStat)
		//	newNode.Id = statementManager.GetNewNodeId()
		//	utils2.InsertBetweenNodes(source[0], ifStatementNode, newNode)
		//}
	}
	for _, next := range allNext {
		ifStatementNode.AddNext(next[1])
		next[1].Source = funk.Filter(next[1].Source, func(item *core.Node) bool {
			return item != next[0]
		}).([]*core.Node)
		next[1].AddSource(ifStatementNode)
	}

	//if lastIfBodyNode != nil && lastIfBodyNode.Next != nil && lastIfBodyNode.Next[0] == ifStatementNode {
	//	if _, ok := lastIfBodyNode.Statement.(*core.GOTOStatement); !ok {
	//		gotoStat := core.NewGOTOStatement()
	//		gotoStat.ToStatement = ifStatementNode.Id
	//		newNode := core.NewNode(gotoStat)
	//		newNode.Id = statementManager.GetNewNodeId()
	//		lastIfBodyNode.Next = []*core.Node{newNode}
	//		ifStatement.IfBody = append(ifStatement.IfBody, newNode.Statement)
	//	}
	//}
	//if lastElseBodyNode != nil && lastElseBodyNode.Next != nil && lastElseBodyNode.Next[0] == ifStatementNode {
	//	if _, ok := lastElseBodyNode.Statement.(*core.GOTOStatement); !ok {
	//		gotoStat := core.NewGOTOStatement()
	//		gotoStat.ToStatement = ifStatementNode.Id
	//		newNode := core.NewNode(gotoStat)
	//		newNode.Id = statementManager.GetNewNodeId()
	//		lastElseBodyNode.Next = []*core.Node{newNode}
	//		ifStatement.ElseBody = append(ifStatement.ElseBody, newNode.Statement)
	//	}
	//}
	//mergeNode.Source = append(mergeNode.Source, ifStatementNode)
	return nil
}
func _RewriteIf(statementManager *StatementManager, node *core.Node) error {
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
	//ifNodeRecord := statementManager.RewriterContext.ifChildSet[ifNode.Id]
	ifNodeRecord := utils.NewSet[*core.Node]()
	ifBodyManager.ScanStatement(func(node *core.Node) (error, bool) {
		ifNodeRecord.Add(node)
		return nil, true
	})
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
		if node.Id == 4 {
			print()
		}
		linkNodeFunc := utils2.CutNode(node, ifBodyStartNode)
		err := ifBodyManager.Rewrite()
		linkNodeFunc()
		if err != nil {
			return err
		}
		ifBody, err := ifBodyManager.ToStatements(func(node *core.Node) bool {
			if checkStartAndEnd(node) {
				return false
			}
			return true
		})
		if err != nil {
			return err
		}
		ifBodyNodes = ifBody
	}
	if elseBodyStartNode != mergeNode {
		linkNodeFunc := utils2.CutNode(node, elseBodyStartNode)
		err := elseBodyManager.Rewrite()
		linkNodeFunc()
		if err != nil {
			return err
		}
		elseBody, err := elseBodyManager.ToStatements(func(node *core.Node) bool {
			if checkStartAndEnd(node) {
				return false
			}
			return true
		})
		if err != nil {
			return err
		}
		elseBodyNodes = elseBody
	}
	// if body must be terminal with goto statement
	lastIfBodyNode := utils.GetLastElement(ifBodyNodes)
	lastElseBodyNode := utils.GetLastElement(elseBodyNodes)
	if lastIfBodyNode != nil { // delete goto statement

		// because if statement all can jmp code block, so terminal with goto statement is not necessary
		//if _, ok := lastIfBodyNode.Statement.(*core.GOTOStatement); !ok {
		//	return errors.New("if body must be terminal with goto statement")
		//}

		if _, ok := lastIfBodyNode.Statement.(*core.GOTOStatement); ok && lastIfBodyNode.Next[0] == mergeNode { // if body default action is jmp to merge node
			ifBodyNodes = ifBodyNodes[:len(ifBodyNodes)-1]
			statementManager.DeleteStatementById(lastIfBodyNode.Id)
		}
	}

	// entire if statement is an entity, need to find if statement source and target
	ifBodyStatements := utils2.NodesToStatements(ifBodyNodes)
	elseBodyStatements := utils2.NodesToStatements(elseBodyNodes)
	allStatementNodeSet := utils.NewSet[*core.Node]()
	allStatementNodeSet.Add(ifNode)
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
		ifStatementNode.AddSource(source[0])
		source[0].Next = funk.Filter(source[0].Next, func(item *core.Node) bool {
			return item != source[1]
		}).([]*core.Node)
		source[0].AddNext(ifStatementNode)
		if source[1] != ifNode {
			gotoStat := core.NewGOTOStatement()
			gotoStat.ToStatement = source[1].Id
			newNode := core.NewNode(gotoStat)
			newNode.Id = statementManager.GetNewNodeId()
			utils2.InsertBetweenNodes(source[0], ifStatementNode, newNode)
		}
	}
	for _, next := range allNext {
		ifStatementNode.AddNext(next[1])
		next[1].Source = funk.Filter(next[1].Source, func(item *core.Node) bool {
			return item != next[0]
		}).([]*core.Node)
		next[1].AddSource(ifStatementNode)
	}

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
