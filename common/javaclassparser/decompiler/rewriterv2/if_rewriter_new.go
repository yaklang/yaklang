package rewriter

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/utils"
)

type rewriterFunc func(statementManager *StatementManager) error

func IfRewriter(manager *StatementManager) error {
	for _, node := range manager.IfNodes {
		node := node
		ifSt := statements.NewIfStatement(node.Statement.(*statements.ConditionStatement).Condition, nil, nil)
		node.Statement = ifSt
		newNode := manager.NewNode(ifSt)
		for _, source := range node.Source {
			for i, n := range source.Next {
				if n == node {
					source.Next[i] = newNode
					node.RemoveSource(source)
				}
			}
		}
		for _, nodes := range manager.DominatorTree {
			for i, n := range nodes {
				if n == node {
					nodes[i] = newNode
				}
			}
		}
		manager.AddFinalAction(func() error {
			getBody := func(node *core.Node) ([]statements.Statement, error) {
				body := []statements.Statement{}
				err := core.WalkGraph[*core.Node](node, func(node *core.Node) ([]*core.Node, error) {
					body = append(body, node.Statement)
					domNextSet := utils.NewSet[*core.Node](manager.DominatorTree[node])
					next := funk.Filter(node.Next, func(n *core.Node) bool {
						return domNextSet.Has(n)
					}).([]*core.Node)
					return next, nil
				})
				if err != nil {
					return nil, err
				}
				return body, nil
			}
			for _, c := range manager.DominatorTree[node] {
				if c == node.Next[1] {
					ifBody, err := getBody(node.Next[1])
					if err != nil {
						return err
					}
					ifSt.IfBody = ifBody
				} else if c == node.Next[0] {
					elseBody, err := getBody(node.Next[0])
					if err != nil {
						return err
					}
					ifSt.ElseBody = elseBody
				}
			}
			return nil
		})
	}
	return nil
}
