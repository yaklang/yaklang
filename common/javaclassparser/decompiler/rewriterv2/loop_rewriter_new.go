package rewriter

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	types2 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/utils"
)

func LoopRewriter(manager *StatementManager) error {
	for _, node := range manager.CircleEntryPoint {
		node := node
		//newStatement := *node
		//newStatementP := &newStatement
		whileSt := statements.NewWhileStatement(values.NewJavaLiteral(true, types2.NewJavaPrimer(types2.JavaBoolean)), nil)
		newNode := manager.NewNode(whileSt)
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
			err := core.WalkGraph[*core.Node](node, func(node *core.Node) ([]*core.Node, error) {
				whileSt.Body = append(whileSt.Body, node.Statement)
				domNextSet := utils.NewSet[*core.Node](manager.DominatorTree[node])
				next := funk.Filter(node.Next, func(n *core.Node) bool {
					return domNextSet.Has(n)
				}).([]*core.Node)
				return next, nil
			})
			if err != nil {
				return err
			}
			return nil
		})
	}
	return nil
}
