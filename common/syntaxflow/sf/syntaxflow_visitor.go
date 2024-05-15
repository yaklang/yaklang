// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package sf // SyntaxFlow
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by SyntaxFlowParser.
type SyntaxFlowVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by SyntaxFlowParser#flow.
	VisitFlow(ctx *FlowContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#filters.
	VisitFilters(ctx *FiltersContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#filterStatement.
	VisitFilterStatement(ctx *FilterStatementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#refVariable.
	VisitRefVariable(ctx *RefVariableContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#RegexpLiteralFilter.
	VisitRegexpLiteralFilter(ctx *RegexpLiteralFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FieldFilter.
	VisitFieldFilter(ctx *FieldFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FieldCallFilter.
	VisitFieldCallFilter(ctx *FieldCallFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FunctionCallFilter.
	VisitFunctionCallFilter(ctx *FunctionCallFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#CurrentRootFilter.
	VisitCurrentRootFilter(ctx *CurrentRootFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#NextFilter.
	VisitNextFilter(ctx *NextFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#OptionalFilter.
	VisitOptionalFilter(ctx *OptionalFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#PrimaryFilter.
	VisitPrimaryFilter(ctx *PrimaryFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#DeepNextFilter.
	VisitDeepNextFilter(ctx *DeepNextFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#ConfiggedDeepNextFilter.
	VisitConfiggedDeepNextFilter(ctx *ConfiggedDeepNextFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#WildcardFilter.
	VisitWildcardFilter(ctx *WildcardFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FieldIndexFilter.
	VisitFieldIndexFilter(ctx *FieldIndexFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#NamedParam.
	VisitNamedParam(ctx *NamedParamContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#EmptyParam.
	VisitEmptyParam(ctx *EmptyParamContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#recursiveConfig.
	VisitRecursiveConfig(ctx *RecursiveConfigContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#recursiveConfigItem.
	VisitRecursiveConfigItem(ctx *RecursiveConfigItemContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#sliceCallItem.
	VisitSliceCallItem(ctx *SliceCallItemContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#nameFilter.
	VisitNameFilter(ctx *NameFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Flat.
	VisitFlat(ctx *FlatContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#BuildMap.
	VisitBuildMap(ctx *BuildMapContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionString.
	VisitFilterExpressionString(ctx *FilterExpressionStringContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionOr.
	VisitFilterExpressionOr(ctx *FilterExpressionOrContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionParen.
	VisitFilterExpressionParen(ctx *FilterExpressionParenContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionAnd.
	VisitFilterExpressionAnd(ctx *FilterExpressionAndContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionCompare.
	VisitFilterExpressionCompare(ctx *FilterExpressionCompareContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionRegexpMatch.
	VisitFilterExpressionRegexpMatch(ctx *FilterExpressionRegexpMatchContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionNumber.
	VisitFilterExpressionNumber(ctx *FilterExpressionNumberContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionRegexp.
	VisitFilterExpressionRegexp(ctx *FilterExpressionRegexpContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionNot.
	VisitFilterExpressionNot(ctx *FilterExpressionNotContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#numberLiteral.
	VisitNumberLiteral(ctx *NumberLiteralContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#stringLiteral.
	VisitStringLiteral(ctx *StringLiteralContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#regexpLiteral.
	VisitRegexpLiteral(ctx *RegexpLiteralContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#identifier.
	VisitIdentifier(ctx *IdentifierContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#types.
	VisitTypes(ctx *TypesContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#boolLiteral.
	VisitBoolLiteral(ctx *BoolLiteralContext) interface{}
}
