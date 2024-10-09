package rewriter

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
)

func LoopRewriter(manager *StatementManager, node *core.Node) error {
	if node.Next[0].Id >= node.Id {
		return nil
	}
	//gotoNode := node
	startNode := node.Next[0]
	if startNode.Next[0].Id <= startNode.Id || startNode.Next[1].Id <= startNode.Id {
		return nil
	}
	whileNext := make([]*core.Node, len(startNode.Next))
	copy(whileNext, startNode.Next)
	whileEndNode := whileNext[0]
	//TODO: found condition statement
	conditionNode, ok := startNode.Statement.(*core.ConditionStatement)
	if !ok {
		return nil
	}
	whileBodyEnd := core.NewMiddleStatement("", "while end")
	whileBodyEndNode := core.NewNode(whileBodyEnd)
	whileBodyEndNode.Id = manager.GetNewNodeId()
	subManager := NewStatementManager(whileNext[1], manager)
	subManager.ScanStatement(func(node *core.Node) (error, bool) {
		for _, n2 := range node.Next {
			if n2.Id < node.Id || n2.Id == whileEndNode.Id {
				node.RemoveNext(n2)
				node.AddNext(whileBodyEndNode)
			}
		}
		if node == whileBodyEndNode {
			return nil, false
		}
		return nil, true
	})
	startNode.RemoveNext(whileEndNode)
	subManager = NewStatementManager(whileNext[1], manager)
	subManager.RewriterContext.BlockStack.Push("while")
	err := subManager.Rewrite()
	if err != nil {
		return err
	}
	subManager.RewriterContext.BlockStack.Pop()
	body, err := subManager.ToStatements(func(node *core.Node) bool {
		return true
	})
	if err != nil {
		return err
	}
	whileStatement := core.NewWhileStatement(conditionNode.Condition, utils.NodesToStatements(body))
	whileNode := core.NewNode(whileStatement)
	whileNode.Id = node.Id
	whileNode.Source = startNode.Source
	whileNode.AddNext(whileEndNode)
	for _, n := range whileNode.Source {
		n.Next = funk.Map(n.Next, func(item *core.Node) *core.Node {
			if item == startNode {
				return whileNode
			}
			return item
		}).([]*core.Node)
	}
	return nil
}
