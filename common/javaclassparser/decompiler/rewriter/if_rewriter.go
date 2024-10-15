package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
)

type rewriterFunc func(statementManager *StatementManager) error

func IfRewriter(manager *StatementManager) error {
	for _, ifNode := range manager.IfNodes {
		ifNode := ifNode
		ifStatement := statements.NewIfStatement(ifNode.Statement.(*statements.ConditionStatement).Condition, nil, nil)
		ifNode.Statement = ifStatement
		trueNode := ifNode.TrueNode()
		falseNode := ifNode.FalseNode()
		ifNode.RemoveAllNext()
		if ifNode.MergeNode != nil && !manager.LoopOccupiedNodes.Has(ifNode.MergeNode) {
			ifNode.AddNext(ifNode.MergeNode)
		}
		manager.AddFinalAction(func() error {
			trueBody, err := manager.ToStatementsFromNode(trueNode, func(node *core.Node) bool {
				if node == ifNode.MergeNode {
					return false
				}
				return true
			})
			if err != nil {
				return err
			}
			falseBody, err := manager.ToStatementsFromNode(falseNode, func(node *core.Node) bool {
				if node == ifNode.MergeNode {
					return false
				}
				return true
			})
			if err != nil {
				return err
			}
			ifStatement.IfBody = core.NodesToStatements(trueBody)
			ifStatement.ElseBody = core.NodesToStatements(falseBody)
			return nil
		})
		//ifNode.AddNext(ifNode.MergeNode)
	}
	return nil
}
