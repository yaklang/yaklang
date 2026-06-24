package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"golang.org/x/exp/slices"
)

func SynchronizeRewriter(manager *RewriteManager, node *core.Node) error {
	val := node.Statement.(*statements.MiddleStatement).Data.(values.JavaValue)
	// Find the TryCatchStatement following the monitor_enter. In rare cases the
	// monitor_enter may have multiple Next nodes (from CFG restructuring); search
	// all of them for a try-catch node.
	var tryNode *core.Node
	var trySt *statements.TryCatchStatement
	for _, n := range node.Next {
		if tc, ok := n.Statement.(*statements.TryCatchStatement); ok {
			trySt = tc
			tryNode = n
			break
		}
	}
	if trySt == nil {
		// No try-catch found — the synchronized pattern is non-standard. Emit a
		// synchronized block with an empty body and continue, rather than failing.
		synNode := manager.NewNode(statements.NewSynchronizedStatement(val, nil))
		for _, s := range node.Source {
			s.ReplaceNext(node, synNode)
		}
		for _, n := range node.Next {
			synNode.AddNext(n)
		}
		return nil
	}
	currentNode := tryNode
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
