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

	// Visit a parse tree produced by SyntaxFlowParser#FilterExecution.
	VisitFilterExecution(ctx *FilterExecutionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterAssert.
	VisitFilterAssert(ctx *FilterAssertContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Description.
	VisitDescription(ctx *DescriptionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#EmptyStatement.
	VisitEmptyStatement(ctx *EmptyStatementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#eos.
	VisitEos(ctx *EosContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionStatement.
	VisitDescriptionStatement(ctx *DescriptionStatementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionItems.
	VisitDescriptionItems(ctx *DescriptionItemsContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionItem.
	VisitDescriptionItem(ctx *DescriptionItemContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#assertStatement.
	VisitAssertStatement(ctx *AssertStatementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#thenExpr.
	VisitThenExpr(ctx *ThenExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#elseExpr.
	VisitElseExpr(ctx *ElseExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#refVariable.
	VisitRefVariable(ctx *RefVariableContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#TopDefSingleFilter.
	VisitTopDefSingleFilter(ctx *TopDefSingleFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FunctionCallFilter.
	VisitFunctionCallFilter(ctx *FunctionCallFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#NextSingleFilter.
	VisitNextSingleFilter(ctx *NextSingleFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#CurrentRootFilter.
	VisitCurrentRootFilter(ctx *CurrentRootFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#NextFilter.
	VisitNextFilter(ctx *NextFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#OptionalFilter.
	VisitOptionalFilter(ctx *OptionalFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#PrimaryFilter.
	VisitPrimaryFilter(ctx *PrimaryFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#ConfiggedDeepNextSingleFilter.
	VisitConfiggedDeepNextSingleFilter(ctx *ConfiggedDeepNextSingleFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#TopDefFilter.
	VisitTopDefFilter(ctx *TopDefFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#ConfiggedTopDefSingleFilter.
	VisitConfiggedTopDefSingleFilter(ctx *ConfiggedTopDefSingleFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#ConfiggedTopDefFilter.
	VisitConfiggedTopDefFilter(ctx *ConfiggedTopDefFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FieldIndexFilter.
	VisitFieldIndexFilter(ctx *FieldIndexFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#DefFilter.
	VisitDefFilter(ctx *DefFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FieldFilter.
	VisitFieldFilter(ctx *FieldFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#DeepNextSingleFilter.
	VisitDeepNextSingleFilter(ctx *DeepNextSingleFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#UseDefCalcFilter.
	VisitUseDefCalcFilter(ctx *UseDefCalcFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FieldCallFilter.
	VisitFieldCallFilter(ctx *FieldCallFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#DefSingleFilter.
	VisitDefSingleFilter(ctx *DefSingleFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#DeepNextFilter.
	VisitDeepNextFilter(ctx *DeepNextFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#ConfiggedDeepNextFilter.
	VisitConfiggedDeepNextFilter(ctx *ConfiggedDeepNextFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#useDefCalcDescription.
	VisitUseDefCalcDescription(ctx *UseDefCalcDescriptionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#useDefCalcParams.
	VisitUseDefCalcParams(ctx *UseDefCalcParamsContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#AllParam.
	VisitAllParam(ctx *AllParamContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#EveryParam.
	VisitEveryParam(ctx *EveryParamContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#actualParamFilter.
	VisitActualParamFilter(ctx *ActualParamFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#singleParam.
	VisitSingleParam(ctx *SingleParamContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#recursiveConfig.
	VisitRecursiveConfig(ctx *RecursiveConfigContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#recursiveConfigItem.
	VisitRecursiveConfigItem(ctx *RecursiveConfigItemContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#recursiveConfigItemValue.
	VisitRecursiveConfigItemValue(ctx *RecursiveConfigItemValueContext) interface{}

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
