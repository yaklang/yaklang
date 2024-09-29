package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)

const (
	IfWriter = 1 << iota
	ForWriter
	SynchronizedWriter
	WhileWriter
	DoWhileWriter
	TryWriter
	SwitchWriter
	TernaryWriter
)

type rewriter struct {
	rewriterFunc   rewriterFunc
	checkStartNode func(node *core.Node, manager *StatementManager) bool
}

var rewriters = map[int]*rewriter{}

func RegisterRewriter(writerType int, rewriterFunc rewriterFunc, checkStartNode func(node *core.Node, manager *StatementManager) bool) {
	rewriters[writerType] = &rewriter{
		rewriterFunc:   rewriterFunc,
		checkStartNode: checkStartNode,
	}
}
func init() {
	RegisterRewriter(IfWriter, RewriteIf, func(node *core.Node, manager *StatementManager) bool {
		_, ok := node.Statement.(*core.ConditionStatement)
		return ok
	})
	RegisterRewriter(SwitchWriter, SwitchRewriter, func(node *core.Node, manager *StatementManager) bool {
		if v, ok := node.Statement.(*core.MiddleStatement); ok && v.Flag == core.MiddleSwitch {
			return true
		} else {
			return false
		}
	})
}
