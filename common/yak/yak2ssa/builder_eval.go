package yak2ssa

import (
	"fmt"
	"strconv"

	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// tryBuildConstantEval handles eval("literal") at SSA build time. The string
// is parsed and built inline in the current scope, so the AOT runtime does not
// need an embedded interpreter for the common constant-string form. Returns
// true when the call was consumed (whether or not the inline build succeeded).
func (b *astbuilder) tryBuildConstantEval(calleeExpr *yak.ExpressionContext, stmt *yak.FunctionCallContext) bool {
	if calleeExpr == nil || calleeExpr.GetText() != "eval" {
		return false
	}
	args, ok := stmt.OrdinaryArguments().(*yak.OrdinaryArgumentsContext)
	if !ok || args == nil || len(args.AllExpression()) != 1 {
		return false
	}
	argExpr, ok := args.Expression(0).(*yak.ExpressionContext)
	if !ok || argExpr == nil {
		return false
	}
	lit, ok := argExpr.Literal().(*yak.LiteralContext)
	if !ok || lit == nil || lit.StringLiteral() == nil {
		return false
	}
	code, err := strconv.Unquote(lit.StringLiteral().GetText())
	if err != nil {
		return false
	}
	ast, err := FrontEnd(code, nil)
	if err != nil {
		b.NewError(ssa.Error, TAG, fmt.Sprintf("eval: parse %q failed: %v", code, err))
		return true
	}
	b.build(ast)
	return true
}
