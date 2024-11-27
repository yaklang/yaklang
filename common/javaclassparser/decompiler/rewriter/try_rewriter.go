package rewriter

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"slices"
)

func TryRewriter(manager *RewriteManager, node *core.Node) error {
	next := make([]*core.Node, len(node.Next))
	copy(next, node.Next)
	tryCatchSt := statements.NewTryCatchStatement(nil, nil)
	tryNode := manager.NewNode(tryCatchSt)
	node.Replace(tryNode)
	tryNode.RemoveAllNext()
	var endNodes []*core.Node
	getBody := func(startNode *core.Node) ([]statements.Statement, error) {
		var sts []statements.Statement
		err := core.WalkGraph[*core.Node](startNode, func(node *core.Node) ([]*core.Node, error) {
			err := manager.CheckVisitedNode(node)
			if err != nil {
				return nil, err
			}
			sts = append(sts, node.Statement)
			var next []*core.Node
			for _, n := range node.Next {
				if slices.Contains(manager.DominatorMap[node], n) {
					next = append(next, n)
				} else {
					endNodes = append(endNodes, n)
				}
			}
			return next, nil
		})
		if err != nil {
			return nil, err
		}
		return sts, nil
	}
	tryBody, err := getBody(next[0])
	if err != nil {
		return err
	}
	catchBodies := [][]statements.Statement{}
	for i := 1; i < len(next); i++ {
		catchBody, err := getBody(next[i])
		if err != nil {
			return err
		}
		catchBodies = append(catchBodies, catchBody)
	}
	for i, body := range catchBodies {
		if len(body) > 0 {
			if v, ok := body[0].(*statements.AssignStatement); ok {
				if v1, ok := v.LeftValue.(*values.JavaRef); ok {
					tryCatchSt.Exception = append(tryCatchSt.Exception, v1)
					catchBodies[i] = body[1:]
				}
			}
		}
	}

	tryCatchSt.TryBody = append(tryCatchSt.TryBody, tryBody...)
	tryCatchSt.CatchBodies = append(tryCatchSt.CatchBodies, catchBodies...)
	endNodes = lo.Filter(endNodes, func(item *core.Node, index int) bool {
		return !IsEndNode(item)
	})
	for _, c := range NodeDeduplication(endNodes) {
		tryNode.AddNext(c)
	}
	return nil
}
