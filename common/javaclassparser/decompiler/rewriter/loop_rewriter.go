package rewriter

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
)

type LoopStatement struct {
	Condition core.JavaValue
	BodyStart *core.Node
}

func LoopRewriter(manager *StatementManager, node *core.Node) error {
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
					statement := conditionNode.Statement.(*core.ConditionStatement)
					statement.Op = core.GetReverseOp(statement.Op)
					if exp, ok := statement.Condition.(*core.JavaExpression); ok {
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
				continueNode := manager.NewNode(core.NewCustomStatement(func(funcCtx *core.FunctionContext) string {
					return "continue"
				}))
				sourceNode.ReplaceNext(conditionNode, continueNode)

				//continueNode.AddNext(loopBodyEnd)
				//sourceNode.AddNext(continueNode)
			}
		}
		outMergeNodeSource := copyNodes(node.OutPointMergeNode.Source)
		for _, sourceNode := range outMergeNodeSource {
			continueNode := manager.NewNode(core.NewCustomStatement(func(funcCtx *core.FunctionContext) string {
				return "break"
			}))
			sourceNode.ReplaceNext(node.OutPointMergeNode, continueNode)
			//continueNode.AddNext(loopBodyEnd)
			//sourceNode.AddNext(continueNode)
		}
		node.OutPointMergeNode.RemoveAllSource()
		var loopCondition core.JavaValue
		for _, n := range loopConditionNode {
			condition := n.Statement.(*core.ConditionStatement).Condition
			if loopCondition == nil {
				loopCondition = condition
			} else {
				loopCondition = core.NewBinaryExpression(loopCondition, condition, core.LOGICAL_OR)
			}
		}
		if loopCondition == nil {
			loopCondition = core.NewJavaLiteral(true, core.JavaBoolean)
		}
		var loopStatement core.Statement
		var setBody func([]core.Statement)
		if _, ok := node.Statement.(*core.ConditionStatement); ok {
			whileStatement := core.NewWhileStatement(loopCondition, nil)
			setBody = func(body []core.Statement) {
				whileStatement.Body = body
			}
			loopStatement = whileStatement
		} else {
			doWhileStatement := core.NewDoWhileStatement(loopCondition, nil)
			setBody = func(body []core.Statement) {
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
		println(utils.DumpNodesToDotExp(firstNode))
		manager.AddFinalAction(func() error {
			body, err := manager.ToStatementsFromNode(firstNode, nil)
			if err != nil {
				return err
			}
			setBody(utils.NodesToStatements(body))
			return nil
		})
	}
	return nil
}
func _LoopRewriter(manager *StatementManager, node *core.Node) error {
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
