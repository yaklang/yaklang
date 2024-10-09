package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)

const (
	IfRewriterFlag = 1 << iota
	ForRewriterFlag
	SynchronizedRewriterFlag
	LoopRewriterFlag
	WhileRewriterFlag
	DoWhileReWriterFlag
	TryRewriterFlag
	SwitchRewriterFlag
	TernaryRewriterFlag
	BreakRewriterFlag
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
	RegisterRewriter(IfRewriterFlag, RewriteIf, func(node *core.Node, manager *StatementManager) bool {
		_, ok := node.Statement.(*core.ConditionStatement)
		return ok
	})
	RegisterRewriter(SwitchRewriterFlag, SwitchRewriter, func(node *core.Node, manager *StatementManager) bool {
		if v, ok := node.Statement.(*core.MiddleStatement); ok && v.Flag == core.MiddleSwitch {
			return true
		} else {
			return false
		}
	})
	RegisterRewriter(LoopRewriterFlag, LoopRewriter, func(node *core.Node, manager *StatementManager) bool {
		if _, ok := node.Statement.(*core.GOTOStatement); ok {
			return true
		} else {
			return false
		}
	})
	RegisterRewriter(DoWhileReWriterFlag, DoWhileRewriter, func(node *core.Node, manager *StatementManager) bool {
		if _, ok := node.Statement.(*core.ConditionStatement); ok {
			return true
		} else {
			return false
		}
	})
	RegisterRewriter(BreakRewriterFlag, BreakRewriter, func(node *core.Node, manager *StatementManager) bool {
		if _, ok := node.Statement.(*core.GOTOStatement); ok {
			return true
		} else {
			return false
		}
	})
}
