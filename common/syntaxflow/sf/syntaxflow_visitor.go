// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package sf // SyntaxFlow
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by SyntaxFlowParser.
type SyntaxFlowVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by SyntaxFlowParser#flow.
	VisitFlow(ctx *FlowContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#statements.
	VisitStatements(ctx *StatementsContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Filter.
	VisitFilter(ctx *FilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Check.
	VisitCheck(ctx *CheckContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Description.
	VisitDescription(ctx *DescriptionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Empty.
	VisitEmpty(ctx *EmptyContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#RefFilterExpr.
	VisitRefFilterExpr(ctx *RefFilterExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#PureFilterExpr.
	VisitPureFilterExpr(ctx *PureFilterExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#eos.
	VisitEos(ctx *EosContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#line.
	VisitLine(ctx *LineContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionStatement.
	VisitDescriptionStatement(ctx *DescriptionStatementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionItems.
	VisitDescriptionItems(ctx *DescriptionItemsContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionItem.
	VisitDescriptionItem(ctx *DescriptionItemContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#checkStatement.
	VisitCheckStatement(ctx *CheckStatementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#thenExpr.
	VisitThenExpr(ctx *ThenExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#elseExpr.
	VisitElseExpr(ctx *ElseExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#refVariable.
	VisitRefVariable(ctx *RefVariableContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#NamedFilter.
	VisitNamedFilter(ctx *NamedFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FieldCallFilter.
	VisitFieldCallFilter(ctx *FieldCallFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#First.
	VisitFirst(ctx *FirstContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FunctionCallFilter.
	VisitFunctionCallFilter(ctx *FunctionCallFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FieldIndexFilter.
	VisitFieldIndexFilter(ctx *FieldIndexFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#OptionalFilter.
	VisitOptionalFilter(ctx *OptionalFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#NextFilter.
	VisitNextFilter(ctx *NextFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#DefFilter.
	VisitDefFilter(ctx *DefFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#DeepNextFilter.
	VisitDeepNextFilter(ctx *DeepNextFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#DeepNextConfigFilter.
	VisitDeepNextConfigFilter(ctx *DeepNextConfigFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#TopDefFilter.
	VisitTopDefFilter(ctx *TopDefFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#TopDefConfigFilter.
	VisitTopDefConfigFilter(ctx *TopDefConfigFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#UseDefCalcFilter.
	VisitUseDefCalcFilter(ctx *UseDefCalcFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#filterExpr.
	VisitFilterExpr(ctx *FilterExprContext) interface{}

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

	// Visit a parse tree produced by SyntaxFlowParser#stringLiteralWithoutStarGroup.
	VisitStringLiteralWithoutStarGroup(ctx *StringLiteralWithoutStarGroupContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#negativeCondition.
	VisitNegativeCondition(ctx *NegativeConditionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#NotCondition.
	VisitNotCondition(ctx *NotConditionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#ParenCondition.
	VisitParenCondition(ctx *ParenConditionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterCondition.
	VisitFilterCondition(ctx *FilterConditionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#OpcodeTypeCondition.
	VisitOpcodeTypeCondition(ctx *OpcodeTypeConditionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionOr.
	VisitFilterExpressionOr(ctx *FilterExpressionOrContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionAnd.
	VisitFilterExpressionAnd(ctx *FilterExpressionAndContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionCompare.
	VisitFilterExpressionCompare(ctx *FilterExpressionCompareContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FilterExpressionRegexpMatch.
	VisitFilterExpressionRegexpMatch(ctx *FilterExpressionRegexpMatchContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#StringContainAnyCondition.
	VisitStringContainAnyCondition(ctx *StringContainAnyConditionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#StringContainHaveCondition.
	VisitStringContainHaveCondition(ctx *StringContainHaveConditionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#numberLiteral.
	VisitNumberLiteral(ctx *NumberLiteralContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#stringLiteral.
	VisitStringLiteral(ctx *StringLiteralContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#stringLiteralWithoutStar.
	VisitStringLiteralWithoutStar(ctx *StringLiteralWithoutStarContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#regexpLiteral.
	VisitRegexpLiteral(ctx *RegexpLiteralContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#identifier.
	VisitIdentifier(ctx *IdentifierContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#keywords.
	VisitKeywords(ctx *KeywordsContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#opcodes.
	VisitOpcodes(ctx *OpcodesContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#types.
	VisitTypes(ctx *TypesContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#boolLiteral.
	VisitBoolLiteral(ctx *BoolLiteralContext) interface{}
}
