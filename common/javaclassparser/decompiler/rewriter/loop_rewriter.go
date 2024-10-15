package rewriter

import (
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
	for _, node := range manager.CirclePoint {
		loopConditionNode := []*core.Node{}
		outNode := []*core.Node{}
		for _, conditionNode := range node.ConditionNode {
			falseNode := conditionNode.FalseNode()
			trueNode := conditionNode.TrueNode()
			if conditionNode == node && falseNode == node.OutPointMergeNode ||
				trueNode == node && falseNode == node.OutPointMergeNode ||
				falseNode == node && trueNode == node.OutPointMergeNode {
				loopConditionNode = append(loopConditionNode, conditionNode)
				if falseNode == node && trueNode == node.OutPointMergeNode {
					statement := conditionNode.Statement.(*statements.ConditionStatement)
					statement.Op = core.GetReverseOp(statement.Op)
					if exp, ok := statement.Condition.(*values.JavaExpression); ok {
						exp.Op = statement.Op
					}
				}
			} else {
				outNode = append(outNode, conditionNode)
			}
		}
		copyNodes := func(nodes []*core.Node) []*core.Node {
			result := make([]*core.Node, len(nodes))
			copy(result, nodes)
			return result
		}

		for _, conditionNode := range loopConditionNode {
			conditionNodeSource := copyNodes(conditionNode.Source)
			for _, sourceNode := range conditionNodeSource {
				if !node.CircleNodesSet.Has(sourceNode) {
					continue
				}
				continueNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.FunctionContext) string {
					return "continue"
				}))
				sourceNode.ReplaceNext(conditionNode, continueNode)

				//continueNode.AddNext(loopBodyEnd)
				//sourceNode.AddNext(continueNode)
			}
		}
		outMergeNodeSource := copyNodes(node.OutPointMergeNode.Source)
		for _, sourceNode := range outMergeNodeSource {
			continueNode := manager.NewNode(statements.NewCustomStatement(func(funcCtx *class_context.FunctionContext) string {
				return "break"
			}))
			sourceNode.ReplaceNext(node.OutPointMergeNode, continueNode)
			//continueNode.AddNext(loopBodyEnd)
			//sourceNode.AddNext(continueNode)
		}
		node.OutPointMergeNode.RemoveAllSource()
		var loopCondition values.JavaValue
		for _, n := range loopConditionNode {
			condition := n.Statement.(*statements.ConditionStatement).Condition
			if loopCondition == nil {
				loopCondition = condition
			} else {
				loopCondition = values.NewBinaryExpression(loopCondition, condition, core.LOGICAL_OR)
			}
		}
		if loopCondition == nil {
			loopCondition = values.NewJavaLiteral(true, types.JavaBoolean)
		}
		var loopStatement statements.Statement
		var setBody func([]statements.Statement)
		if _, ok := node.Statement.(*statements.ConditionStatement); ok {
			whileStatement := statements.NewWhileStatement(loopCondition, nil)
			setBody = func(body []statements.Statement) {
				whileStatement.Body = body
			}
			loopStatement = whileStatement
		} else {
			doWhileStatement := statements.NewDoWhileStatement(loopCondition, nil)
			setBody = func(body []statements.Statement) {
				doWhileStatement.Body = body
			}
			loopStatement = doWhileStatement
		}
		originNodeNext := make([]*core.Node, len(node.Next))
		copy(originNodeNext, node.Next)
		node.Statement = loopStatement
		node.RemoveAllNext()
		node.AddNext(node.OutPointMergeNode)
		manager.LoopOccupiedNodes.Add(node.OutPointMergeNode)
		var firstNode *core.Node
		for _, n := range originNodeNext {
			if node.CircleNodesSet.Has(n) {
				firstNode = n
				break
			}
		}
		manager.AddFinalAction(func() error {
			body, err := manager.ToStatementsFromNode(firstNode, nil)
			if err != nil {
				return err
			}
			setBody(core.NodesToStatements(body))
			return nil
		})
	}
	return nil
}
