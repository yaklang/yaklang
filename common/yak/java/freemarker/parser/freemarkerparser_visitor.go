// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package freemarkerparser // FreemarkerParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by FreemarkerParser.
type FreemarkerParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by FreemarkerParser#template.
	VisitTemplate(ctx *TemplateContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#elements.
	VisitElements(ctx *ElementsContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#RawTextElement.
	VisitRawTextElement(ctx *RawTextElementContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#DirectiveElement.
	VisitDirectiveElement(ctx *DirectiveElementContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#InlineExprElement.
	VisitInlineExprElement(ctx *InlineExprElementContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#rawText.
	VisitRawText(ctx *RawTextContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directive.
	VisitDirective(ctx *DirectiveContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveIf.
	VisitDirectiveIf(ctx *DirectiveIfContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveIfTrueElements.
	VisitDirectiveIfTrueElements(ctx *DirectiveIfTrueElementsContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveIfElseIfElements.
	VisitDirectiveIfElseIfElements(ctx *DirectiveIfElseIfElementsContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveIfElseElements.
	VisitDirectiveIfElseElements(ctx *DirectiveIfElseElementsContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#tagExprElseIfs.
	VisitTagExprElseIfs(ctx *TagExprElseIfsContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveAssign.
	VisitDirectiveAssign(ctx *DirectiveAssignContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveList.
	VisitDirectiveList(ctx *DirectiveListContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveListBodyElements.
	VisitDirectiveListBodyElements(ctx *DirectiveListBodyElementsContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveListElseElements.
	VisitDirectiveListElseElements(ctx *DirectiveListElseElementsContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveInclude.
	VisitDirectiveInclude(ctx *DirectiveIncludeContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveImport.
	VisitDirectiveImport(ctx *DirectiveImportContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveMacro.
	VisitDirectiveMacro(ctx *DirectiveMacroContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveNested.
	VisitDirectiveNested(ctx *DirectiveNestedContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveReturn.
	VisitDirectiveReturn(ctx *DirectiveReturnContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveUser.
	VisitDirectiveUser(ctx *DirectiveUserContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveUserId.
	VisitDirectiveUserId(ctx *DirectiveUserIdContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveUserParams.
	VisitDirectiveUserParams(ctx *DirectiveUserParamsContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#directiveUserLoopParams.
	VisitDirectiveUserLoopParams(ctx *DirectiveUserLoopParamsContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#tagExpr.
	VisitTagExpr(ctx *TagExprContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#inlineExpr.
	VisitInlineExpr(ctx *InlineExprContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#SingleQuote.
	VisitSingleQuote(ctx *SingleQuoteContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#DoubleQuote.
	VisitDoubleQuote(ctx *DoubleQuoteContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprUnaryOp.
	VisitExprUnaryOp(ctx *ExprUnaryOpContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprMulDivMod.
	VisitExprMulDivMod(ctx *ExprMulDivModContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#BoolExpr.
	VisitBoolExpr(ctx *BoolExprContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#StringExpr.
	VisitStringExpr(ctx *StringExprContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprBoolRelational.
	VisitExprBoolRelational(ctx *ExprBoolRelationalContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprRoundParentheses.
	VisitExprRoundParentheses(ctx *ExprRoundParenthesesContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprBoolAnd.
	VisitExprBoolAnd(ctx *ExprBoolAndContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#SymbolExpr.
	VisitSymbolExpr(ctx *SymbolExprContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprBuiltIn.
	VisitExprBuiltIn(ctx *ExprBuiltInContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#StructExpr.
	VisitStructExpr(ctx *StructExprContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprMissingTest.
	VisitExprMissingTest(ctx *ExprMissingTestContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprAddSub.
	VisitExprAddSub(ctx *ExprAddSubContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprDotAccess.
	VisitExprDotAccess(ctx *ExprDotAccessContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprBoolEq.
	VisitExprBoolEq(ctx *ExprBoolEqContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprFunctionCall.
	VisitExprFunctionCall(ctx *ExprFunctionCallContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#NumberExpr.
	VisitNumberExpr(ctx *NumberExprContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprDefault.
	VisitExprDefault(ctx *ExprDefaultContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprSquareParentheses.
	VisitExprSquareParentheses(ctx *ExprSquareParenthesesContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#ExprBoolOr.
	VisitExprBoolOr(ctx *ExprBoolOrContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#functionParams.
	VisitFunctionParams(ctx *FunctionParamsContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#booleanRelationalOperator.
	VisitBooleanRelationalOperator(ctx *BooleanRelationalOperatorContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#struct.
	VisitStruct(ctx *StructContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#struct_pair.
	VisitStruct_pair(ctx *Struct_pairContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#single_quote_string.
	VisitSingle_quote_string(ctx *Single_quote_stringContext) interface{}

	// Visit a parse tree produced by FreemarkerParser#double_quote_string.
	VisitDouble_quote_string(ctx *Double_quote_stringContext) interface{}
}
