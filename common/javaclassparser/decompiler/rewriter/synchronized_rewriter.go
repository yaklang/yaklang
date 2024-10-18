package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
)

func SynchronizedRewriter(manager *StatementManager, node *core.Node) error {
	if err := manager.ScanStatementSimple(func(node *core.Node) error {
		cStem, ok := node.Statement.(*statements.CustomStatement)
		if !ok {
			return nil
		}
		if cStem.Name != "monitor_enter" {
			return nil
		}
		monitorValue := cStem.Info.(values.JavaValue)
		monitorManger := NewStatementManager(node.Next[0], manager)
		var exitNode *core.Node
		err := monitorManger.Rewrite()
		if err != nil {
			return err
		}
		if exitNode == nil {
			return nil
		}
		body, err := monitorManger.ToStatements(func(node *core.Node) bool {
			if len(node.Next) == 0 {
				return true
			}
			nextNode := node.Next[0]
			cStem, ok := nextNode.Statement.(*statements.CustomStatement)
			if ok && cStem.Name == "monitor_exit" {
				exitNode = nextNode
				return false
			}
			return true
		})
		if err != nil {
			return err
		}
		node.Statement = statements.NewSynchronizedStatement(monitorValue, core.NodesToStatements(body))
		node.Next = exitNode.Next
		if _, ok := exitNode.Next[0].Statement.(*statements.GOTOStatement); ok {
			node.Next = exitNode.Next[0].Next
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}
