package rewriter

import (
	"errors"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type LoopStatement struct {
	Condition values.JavaValue
	BodyStart *core.Node
}

func LoopRewriter(manager *StatementManager) error {
	for _, node := range manager.CircleEntryPoint {
		originNodeNext := make([]*core.Node, len(node.Next))
		copy(originNodeNext, node.Next)
		LoopEndNode := node.GetLoopEndNode()
		loopconditionStat, ok := node.Statement.(*statements.ConditionStatement)
		var entryConditionNode *core.Node
		var loopCondition values.JavaValue
		isWhile := false
		circleSetHas := func(n *core.Node) bool {
			if v, ok := manager.RepeatNodeMap[n]; ok {
				n = v
			}
			return node.CircleNodesSet.Has(n)
		}
		if ok {
			for i, n := range node.Next {
				if !circleSetHas(n) && n == LoopEndNode {
					entryConditionNode = node.Next[1-i]
					loopCondition = loopconditionStat.Condition
					node.IsCircle = true
					isWhile = true
					break
				}
			}
		}
		if !isWhile {
			entryConditionNode = node
			loopCondition = values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean))
		}
		//outNode := []*core.Node{}
		//for _, conditionNode := range node.ConditionNode {
		//	for _, n := range conditionNode.Next {
		//		if n == node.LoopEndNode {
		//			outNode = append(outNode, conditionNode)
		//		}
		//	}
		//	falseNode := conditionNode.FalseNode()
		//	trueNode := conditionNode.TrueNode()
		//	if conditionNode == node && falseNode == node.LoopEndNode ||
		//		trueNode == node && falseNode == node.LoopEndNode ||
		//		falseNode == node && trueNode == node.LoopEndNode {
		//		loopConditionNode = append(loopConditionNode, conditionNode)
		//		if falseNode == node && trueNode == node.LoopEndNode {
		//			statement := conditionNode.Statement.(*statements.ConditionStatement)
		//			statement.Op = core.GetReverseOp(statement.Op)
		//			if exp, ok := statement.Condition.(*values.JavaExpression); ok {
		//				exp.Op = statement.Op
		//			}
		//		}
		//	} else {
		//		outNode = append(outNode, conditionNode)
		//		loopConditionNode = append(loopConditionNode, conditionNode)
		//	}
		//}
		copyNodes := func(nodes []*core.Node) []*core.Node {
			result := make([]*core.Node, len(nodes))
			copy(result, nodes)
			return result
		}
		conditionNodeSource := copyNodes(node.Source)
		continueNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
			return "continue"
		}))
		for _, sourceNode := range conditionNodeSource {
			if !circleSetHas(sourceNode) {
				continue
			}
			if sourceNode.Id == 18 {
				print()
			}
			sourceNode.ReplaceNext(node, continueNode)
			continueNode.AddSource(sourceNode)
			node.RemoveSource(sourceNode)
		}
		node.SetLoopEndNode(node, manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
			return "break"
		})))
		for _, n := range node.Source {
			if circleSetHas(n) {
				return errors.New("cut jmp loop header edge failed")
			}
		}
		//if entryConditionNode != nil {
		//	conditionNodeSource := copyNodes(entryConditionNode.Source)
		//	for _, sourceNode := range conditionNodeSource {
		//		if !circleSetHas(sourceNode) {
		//			continue
		//		}
		//		continueNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
		//			return "continue"
		//		}))
		//		sourceNode.ReplaceNext(entryConditionNode, continueNode)
		//	}
		//}

		//for _, conditionNode := range node.ConditionNode {
		//	if !circleSetHas(conditionNode.Next[0]) && !circleSetHas(conditionNode.Next[1]) {
		//		continue
		//	}
		//	conditionNodeSource := copyNodes(conditionNode.Source)
		//	for _, sourceNode := range conditionNodeSource {
		//		if !circleSetHas(sourceNode) {
		//			continue
		//		}
		//		continueNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
		//			return "continue"
		//		}))
		//		sourceNode.ReplaceNext(conditionNode, continueNode)
		//
		//		//continueNode.AddNext(loopBodyEnd)
		//		//sourceNode.AddNext(continueNode)
		//	}
		//}
		for _, n := range node.BreakNode {
			if isWhile && n == node {
				continue
			}
			breakNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
				return "break"
			}))
			if n.Id == 18 {
				print()
			}
			var endNode *core.Node
			for _, n2 := range n.Next {
				if !node.CircleNodesSet.Has(n2) {
					endNode = n2
					break
				}
			}
			n.ReplaceNext(endNode, breakNode)
			breakNode.AddSource(n)
			LoopEndNode.RemoveSource(n)
		}
		//outMergeNodeSource := copyNodes(node.LoopEndNode.Source)
		//outMergeNodeSource = funk.Filter(outMergeNodeSource, func(item *core.Node) bool {
		//	if entryConditionNode != nil && item == entryConditionNode {
		//		return false
		//	}
		//	//return circleSetHas(item)
		//	return true
		//}).([]*core.Node)
		//occupiedEnd := len(outMergeNodeSource) == len(node.LoopEndNode.Source)
		//outMergeNodeSource1 := []*core.Node{}
		//for _, sourceNode := range outMergeNodeSource {
		//	core.WalkGraph[*core.Node](sourceNode, func(node *core.Node) ([]*core.Node, error) {
		//		for _, n := range node.Next {
		//			if n == node.LoopEndNode {
		//				continueNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.ClassContext) string {
		//					return "break"
		//				}))
		//				n.ReplaceNext(node.LoopEndNode, continueNode)
		//				//outMergeNodeSource1 = append(outMergeNodeSource1, node)
		//				outMergeNodeSource1 = append(outMergeNodeSource1, node)
		//				return nil, nil
		//			}
		//		}
		//		return node.Next, nil
		//	})
		//
		//	//continueNode.AddNext(loopBodyEnd)
		//	//sourceNode.AddNext(continueNode)
		//}
		//for _, n := range outMergeNodeSource1 {
		//	if n == entryConditionNode {
		//		continue
		//	}
		//	node.LoopEndNode.RemoveSource(n)
		//}
		////var loopCondition values.JavaValue
		////for _, n := range node.ConditionNode {
		////	if n.IsCircle {
		////		continue
		////	}
		////	condition := n.Statement.(*statements.ConditionStatement).Condition
		////	if loopCondition == nil {
		////		loopCondition = condition
		////	} else {
		////		loopCondition = values.NewBinaryExpression(loopCondition, condition, core.LOGICAL_OR)
		////	}
		////}
		//loopCondition
		//var loopStatement statements.Statement
		var setBody func([]statements.Statement)
		//isDoWhile := false
		var loopNode *core.Node
		if isWhile {
			whileStatement := statements.NewWhileStatement(loopCondition, nil)
			setBody = func(body []statements.Statement) {
				whileStatement.Body = body
			}
			loopNode = manager.NewNode(whileStatement)
			for _, n := range node.Source {
				loopNode.AddSource(n)
			}
			node.RemoveAllSource()
			node.RemoveAllNext()
			manager.RepeatNodeMap[loopNode] = node
			loopNode.AddNext(LoopEndNode)
			entryConditionNode.RemoveNext(LoopEndNode)
		} else {
			doWhileStatement := statements.NewDoWhileStatement(values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), nil)
			setBody = func(body []statements.Statement) {
				doWhileStatement.Body = body
			}
			loopNode = manager.NewNode(doWhileStatement)
			for _, n := range node.Source {
				loopNode.AddSource(n)
			}
			node.RemoveAllSource()
			loopNode.AddNext(LoopEndNode)
		}

		//node.Statement = whileStatement
		//loopStatement = whileStatement
		//if _, ok := node.Statement.(*statements.ConditionStatement); ok {
		//
		//} else {
		//	isDoWhile = true
		//	doWhileStatement := statements.NewDoWhileStatement(loopCondition, nil)
		//	setBody = func(body []statements.Statement) {
		//		doWhileStatement.Body = body
		//	}
		//	loopStatement = doWhileStatement
		//}
		//firstSt := node.Statement
		//node.Statement = loopStatement
		//node.RemoveAllNext()
		//node.AddNext(node.LoopEndNode)

		//manager.LoopOccupiedNodes.Add(node)
		//if occupiedEnd {
		//	manager.LoopOccupiedNodes.Add(node.LoopEndNode)
		//}
		//var firstNode *core.Node
		//for _, n := range originNodeNext {
		//	if circleSetHas(n) {
		//		firstNode = n
		//		break
		//	}
		//}
		//if firstNode == nil {
		//	return errors.New("first node not found")
		//}
		//node.RemoveNext(firstNode)
		//whileNode.Next = []*core.Node{node.LoopEndNode}
		//if !isWhile {
		//	node.LoopEndNode.RemoveNext()
		//}
		manager.AddFinalAction(func() error {
			println(isWhile)
			body, err := manager.ToStatementsFromNode(entryConditionNode, nil)
			if err != nil {
				return err
			}
			bodyStat := core.NodesToStatements(body)
			//if isDoWhile {
			//	bodyStat = append([]statements.Statement{firstSt}, bodyStat...)
			//}
			setBody(bodyStat)
			return nil
		})
	}
	return nil
}
