package rewriter

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
)

func DoWhileRewriter(manager *StatementManager, node *core.Node) error {
	if len(node.Next) == 2 && (node.Next[0].Id >= node.Id && node.Next[1].Id >= node.Id) {
		return nil
	}
	if len(node.Next) != 2 {
		return nil
	}
	conditionNode := node
	bodyStartNode := node.Next[0]
	bodyEndNode := node.Next[1]
	_ = bodyEndNode
	whileBodyEnd := core.NewMiddleStatement("", "do while end")
	whileBodyEndNode := core.NewNode(whileBodyEnd)
	whileBodyEndNode.Id = manager.GetNewNodeId()
	subManager := NewStatementManager(bodyStartNode, manager)
	subManager.ScanStatement(func(node *core.Node) (error, bool) {
		for _, n2 := range node.Next {
			if n2.Id < node.Id || n2.Id == bodyEndNode.Id {
				node.RemoveNext(n2)
				node.AddNext(whileBodyEndNode)
			}
		}
		if node == whileBodyEndNode {
			return nil, false
		}
		return nil, true
	})
	//TODO: found condition statement
	conditionStatement, ok := conditionNode.Statement.(*core.ConditionStatement)
	if !ok {
		return nil
	}
	node.RemoveAllSource()
	subManager = NewStatementManager(bodyStartNode, manager)
	subManager.RewriterContext.BlockStack.Push("do_while")
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
	whileStatement := core.NewDoWhileStatement(conditionStatement.Condition, utils.NodesToStatements(body))
	whileNode := core.NewNode(whileStatement)
	whileNode.Id = node.Id
	whileNode.Source = bodyStartNode.Source
	whileNode.AddNext(whileBodyEndNode)
	for _, n := range whileNode.Source {
		n.Next = funk.Map(n.Next, func(item *core.Node) *core.Node {
			if item == bodyStartNode {
				return whileNode
			}
			return item
		}).([]*core.Node)
	}
	return nil
}
