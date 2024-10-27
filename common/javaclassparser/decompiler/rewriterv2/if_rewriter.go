package rewriter

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

func _IfRewriter(manager *StatementManager) error {
	for _, ifNode := range manager.IfNodes {
		if ifNode.IsCircle {
			continue
		}
		ifNode := ifNode
		ifStatement := statements.NewIfStatement(ifNode.Statement.(*statements.ConditionStatement).Condition, nil, nil)
		ifNode.Statement = ifStatement
		trueNode := ifNode.TrueNode()
		falseNode := ifNode.FalseNode()
		ifNode.RemoveAllNext()
		var ok1, ok2 bool
		core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
			if node == ifNode.MergeNode {
				ok1 = true
				return nil, nil
			}
			return node.Next, nil
		})
		core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
			if node == ifNode.MergeNode {
				ok2 = true
				return nil, nil
			}
			return node.Next, nil
		})
		if ok1 && ok2 {
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
			if len(ifStatement.IfBody) == 1 && len(ifStatement.ElseBody) == 1 {
				v1, ok1 := ifStatement.IfBody[0].(*statements.StackAssignStatement)
				v2, ok2 := ifStatement.ElseBody[0].(*statements.StackAssignStatement)
				if ok1 && ok2 {
					v2.JavaValue.JavaType.ResetType(v1.JavaValue.Type())
					v1.JavaValue.CustomValue = values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
						return fmt.Sprintf("%s ? %s : %s", ifStatement.Condition.String(funcCtx), v1.JavaValue.StackVar.String(funcCtx), v2.JavaValue.StackVar.String(funcCtx))
					}, func() types.JavaType {
						return v1.JavaValue.Type()
					})
					v2.JavaValue.CustomValue = v1.JavaValue.CustomValue
					allSource := make([]*core.Node, len(ifNode.Source))
					copy(allSource, ifNode.Source)
					for _, source := range allSource {
						source.RemoveNext(ifNode)
						for _, next := range ifNode.Next {
							source.AddNext(next)
						}
					}
				}
			}
			return nil
		})
		//ifNode.AddNext(ifNode.MergeNode)
	}
	return nil
}
