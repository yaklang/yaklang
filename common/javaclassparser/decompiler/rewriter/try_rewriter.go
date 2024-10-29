package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/utils"
)

func TryRewriter(manager *StatementManager) error {
	for _, node := range manager.TryNodes {
		println(node.Next[1].Statement.String(&class_context.ClassContext{}))
		leftSet := utils.NewSet[*core.Node]()
		core.WalkGraph[*core.Node](node.Next[0], func(node *core.Node) ([]*core.Node, error) {
			leftSet.Add(node)
			return node.Next, nil
		})
		var mergeNode *core.Node
		core.WalkGraph[*core.Node](node.Next[1], func(node *core.Node) ([]*core.Node, error) {
			if leftSet.Has(node) {
				mergeNode = node
				return nil, nil
			}
			return node.Next, nil
		})
		node := node
		//if mergeNode == nil {
		//	return errors.New("try rewriter error")
		//}
		next := make([]*core.Node, len(node.Next))
		copy(next, node.Next)
		//node.RemoveAllNext()
		//
		//tryCatchNode := manager.NewNode(tryCatchSt)
		//node.AddNext(tryCatchNode)
		tryCatchSt := statements.NewTryCatchStatement(nil, nil)
		node.Statement = tryCatchSt
		node.RemoveAllNext()
		if mergeNode != nil {
			node.AddNext(mergeNode)
		}
		manager.AddFinalAction(func() error {
			tryBody, err := manager.ToStatementsFromNode(next[0], func(node *core.Node) bool {
				if mergeNode != nil && node == mergeNode {
					return false
				}
				return true
			})
			if err != nil {
				return err
			}
			tryCatchSt.TryBody = core.NodesToStatements(tryBody)
			catchBody, err := manager.ToStatementsFromNode(next[1], func(node *core.Node) bool {
				if mergeNode != nil && node == mergeNode {
					return false
				}
				return true
			})
			if err != nil {
				return err
			}
			tryCatchSt.Exception = catchBody[0].Statement.(*statements.AssignStatement).LeftValue.(*values.JavaRef)
			tryCatchSt.CatchBody = core.NodesToStatements(catchBody)[1:]
			return nil
		})
	}
	return nil
}
