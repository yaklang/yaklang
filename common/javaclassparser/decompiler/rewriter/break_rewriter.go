package rewriter

import "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"

func BreakRewriter(manager *StatementManager, node *core.Node) error {
	if manager.RewriterContext.BlockStack.Peek() != nil {
		node.Statement = core.NewCustomStatement(func(funcCtx *core.FunctionContext) string {
			return "break"
		})
		return nil
	}
	return nil
}
