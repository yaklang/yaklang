// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package sf // SyntaxFlow
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type BaseSyntaxFlowVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseSyntaxFlowVisitor) VisitFlow(ctx *FlowContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilters(ctx *FiltersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExecution(ctx *FilterExecutionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterAssert(ctx *FilterAssertContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDescription(ctx *DescriptionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitEmptyStatement(ctx *EmptyStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitEos(ctx *EosContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDescriptionStatement(ctx *DescriptionStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDescriptionItems(ctx *DescriptionItemsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDescriptionItem(ctx *DescriptionItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitAssertStatement(ctx *AssertStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitThenExpr(ctx *ThenExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitElseExpr(ctx *ElseExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRefVariable(ctx *RefVariableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitTopDefSingleFilter(ctx *TopDefSingleFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFunctionCallFilter(ctx *FunctionCallFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNextSingleFilter(ctx *NextSingleFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitCurrentRootFilter(ctx *CurrentRootFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNextFilter(ctx *NextFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitOptionalFilter(ctx *OptionalFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitPrimaryFilter(ctx *PrimaryFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitConfiggedDeepNextSingleFilter(ctx *ConfiggedDeepNextSingleFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitTopDefFilter(ctx *TopDefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitConfiggedTopDefSingleFilter(ctx *ConfiggedTopDefSingleFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitConfiggedTopDefFilter(ctx *ConfiggedTopDefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFieldIndexFilter(ctx *FieldIndexFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDefFilter(ctx *DefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFieldFilter(ctx *FieldFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDeepNextSingleFilter(ctx *DeepNextSingleFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitUseDefCalcFilter(ctx *UseDefCalcFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFieldCallFilter(ctx *FieldCallFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDefSingleFilter(ctx *DefSingleFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDeepNextFilter(ctx *DeepNextFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitConfiggedDeepNextFilter(ctx *ConfiggedDeepNextFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitUseDefCalcDescription(ctx *UseDefCalcDescriptionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitUseDefCalcParams(ctx *UseDefCalcParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitAllParam(ctx *AllParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitEveryParam(ctx *EveryParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitActualParamFilter(ctx *ActualParamFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitSingleParam(ctx *SingleParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRecursiveConfig(ctx *RecursiveConfigContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRecursiveConfigItem(ctx *RecursiveConfigItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRecursiveConfigItemValue(ctx *RecursiveConfigItemValueContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitSliceCallItem(ctx *SliceCallItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNameFilter(ctx *NameFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFlat(ctx *FlatContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitBuildMap(ctx *BuildMapContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpressionString(ctx *FilterExpressionStringContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpressionOr(ctx *FilterExpressionOrContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpressionParen(ctx *FilterExpressionParenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpressionAnd(ctx *FilterExpressionAndContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpressionCompare(ctx *FilterExpressionCompareContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpressionRegexpMatch(ctx *FilterExpressionRegexpMatchContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpressionNumber(ctx *FilterExpressionNumberContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpressionRegexp(ctx *FilterExpressionRegexpContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpressionNot(ctx *FilterExpressionNotContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNumberLiteral(ctx *NumberLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitStringLiteral(ctx *StringLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRegexpLiteral(ctx *RegexpLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitIdentifier(ctx *IdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitTypes(ctx *TypesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitBoolLiteral(ctx *BoolLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}
