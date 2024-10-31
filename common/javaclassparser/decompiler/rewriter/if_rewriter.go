package rewriter

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"slices"
)

type rewriterFunc func(statementManager *StatementManager) error

func IfRewriter(manager *StatementManager) error {
	for _, ifNode := range manager.IfNodes {
		ifNode := ifNode
		if ifNode.IsCircle {
			continue
		}
		trueNode := ifNode.TrueNode()
		falseNode := ifNode.FalseNode()
		ifNode.RemoveAllNext()
		mergeNode := ifNode.MergeNode
		if mergeNode != nil {
			ifNode.AddNext(mergeNode)
		}
		ifStatement := statements.NewIfStatement(nil, nil, nil)
		originNodeStatement := ifNode.Statement
		ifNode.Statement = ifStatement
		//var mergeNode *core.Node
		//var ok1, ok2 bool
		//core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
		//	if node == ifNode.MergeNode {
		//		ok1 = true
		//		return nil, nil
		//	}
		//	return node.Next, nil
		//})
		//core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
		//	if node == ifNode.MergeNode {
		//		ok2 = true
		//		return nil, nil
		//	}
		//	return node.Next, nil
		//})
		//if ok1 && ok2 {
		//	mergeNode = ifNode.MergeNode
		//}
		//if slices.Contains(manager.DominatorMap[ifNode], ifNode.MergeNode) {
		//	ifNode.AddNext(ifNode.MergeNode)
		//}
		manager.AddFinalAction(func() error {
			getBody := func(bodyStartNode *core.Node) ([]statements.Statement, error) {
				sts := []statements.Statement{}
				if !slices.Contains(manager.DominatorMap[ifNode], bodyStartNode) {
					return sts, nil
				}
				err := core.WalkGraph[*core.Node](bodyStartNode, func(node *core.Node) ([]*core.Node, error) {
					if mergeNode != nil && node == mergeNode {
						return nil, nil
					}
					if len(node.Next) > 1 {
						return nil, fmt.Errorf("invalid if node %d", node.Id)
					}
					sts = append(sts, node.Statement)
					var next []*core.Node
					for _, n := range node.Next {
						if slices.Contains(manager.DominatorMap[node], n) {
							next = append(next, n)
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
			if trueNode != nil {
				ifBody, err := getBody(trueNode)
				if err != nil {
					return err
				}
				ifStatement.IfBody = ifBody
			}
			if falseNode != nil {
				elseBody, err := getBody(falseNode)
				if err != nil {
					return err
				}

				ifStatement.ElseBody = elseBody
			}
			return nil
		})
	}
	return nil
}
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
			if v, ok := ifStatement.Condition.(*values.FunctionCallExpression); ok {
				if len(v.Arguments) == 4 {
					if v, ok := v.Arguments[1].(*values.JavaLiteral); ok {
						if v.Data == "extensions" {
							print()
						}
					}
				}
			}
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
