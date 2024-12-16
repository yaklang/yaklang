package rewriter

import (
	"errors"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"golang.org/x/exp/slices"
)

func SynchronizeRewriter(manager *RewriteManager, node *core.Node) error {
	val := node.Statement.(*statements.MiddleStatement).Data.(values.JavaValue)
	if len(node.Next) != 1 {
		return errors.New("invalid synchronized block")
	}
	currentNode := node.Next[0]
	trySt, ok := currentNode.Statement.(*statements.TryCatchStatement)
	if !ok {
		return errors.New("invalid synchronized block")
	}
	var bodySts, otherBody []statements.Statement
	for i := 0; i < len(trySt.TryBody); i++ {
		if v, ok := trySt.TryBody[i].(*statements.MiddleStatement); ok && v.Flag == "monitor_exit" {
			bodySts = trySt.TryBody[:i]
			otherBody = trySt.TryBody[i+1:]
			break
		}
	}
	next := slices.Clone(currentNode.Next)
	source := slices.Clone(node.Source)
	synNode := manager.NewNode(statements.NewSynchronizedStatement(val, bodySts))
	currentN := synNode
	for _, statement := range otherBody {
		n := manager.NewNode(statement)
		currentN.AddNext(n)
		currentN = n
	}
	nextNode := currentN
	for _, n := range next {
		n.RemoveSource(currentNode)
	}
	for _, n := range next {
		n.AddSource(nextNode)
	}
	for _, n := range source {
		n.ReplaceNext(node, synNode)
	}
	return nil
}
