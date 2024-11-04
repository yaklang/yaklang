// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package freemarkerparser // FreemarkerParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type BaseFreemarkerParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseFreemarkerParserVisitor) VisitTemplate(ctx *TemplateContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitElements(ctx *ElementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitRawTextElement(ctx *RawTextElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveElement(ctx *DirectiveElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitInlineExprElement(ctx *InlineExprElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitRawText(ctx *RawTextContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirective(ctx *DirectiveContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveIf(ctx *DirectiveIfContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveIfTrueElements(ctx *DirectiveIfTrueElementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveIfElseIfElements(ctx *DirectiveIfElseIfElementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveIfElseElements(ctx *DirectiveIfElseElementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitTagExprElseIfs(ctx *TagExprElseIfsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveAssign(ctx *DirectiveAssignContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveList(ctx *DirectiveListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveListBodyElements(ctx *DirectiveListBodyElementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveListElseElements(ctx *DirectiveListElseElementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveInclude(ctx *DirectiveIncludeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveImport(ctx *DirectiveImportContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveMacro(ctx *DirectiveMacroContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveNested(ctx *DirectiveNestedContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveReturn(ctx *DirectiveReturnContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveUser(ctx *DirectiveUserContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveUserId(ctx *DirectiveUserIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveUserParams(ctx *DirectiveUserParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDirectiveUserLoopParams(ctx *DirectiveUserLoopParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitTagExpr(ctx *TagExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitInlineExpr(ctx *InlineExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitSingleQuote(ctx *SingleQuoteContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDoubleQuote(ctx *DoubleQuoteContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprUnaryOp(ctx *ExprUnaryOpContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprMulDivMod(ctx *ExprMulDivModContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitBoolExpr(ctx *BoolExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitStringExpr(ctx *StringExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprBoolRelational(ctx *ExprBoolRelationalContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprRoundParentheses(ctx *ExprRoundParenthesesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprBoolAnd(ctx *ExprBoolAndContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitSymbolExpr(ctx *SymbolExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprBuiltIn(ctx *ExprBuiltInContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitStructExpr(ctx *StructExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprMissingTest(ctx *ExprMissingTestContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprAddSub(ctx *ExprAddSubContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprDotAccess(ctx *ExprDotAccessContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprBoolEq(ctx *ExprBoolEqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprFunctionCall(ctx *ExprFunctionCallContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitNumberExpr(ctx *NumberExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprDefault(ctx *ExprDefaultContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprSquareParentheses(ctx *ExprSquareParenthesesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitExprBoolOr(ctx *ExprBoolOrContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitFunctionParams(ctx *FunctionParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitBooleanRelationalOperator(ctx *BooleanRelationalOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitStruct(ctx *StructContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitStruct_pair(ctx *Struct_pairContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitSingle_quote_string(ctx *Single_quote_stringContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseFreemarkerParserVisitor) VisitDouble_quote_string(ctx *Double_quote_stringContext) interface{} {
	return v.VisitChildren(ctx)
}
