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

func (v *BaseSyntaxFlowVisitor) VisitFilterStatement(ctx *FilterStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRefVariable(ctx *RefVariableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRegexpLiteralFilter(ctx *RegexpLiteralFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFunctionCallFilter(ctx *FunctionCallFilterContext) interface{} {
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

func (v *BaseSyntaxFlowVisitor) VisitTopDefFilter(ctx *TopDefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitConfiggedTopDefFilter(ctx *ConfiggedTopDefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitWildcardFilter(ctx *WildcardFilterContext) interface{} {
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

func (v *BaseSyntaxFlowVisitor) VisitFieldCallFilter(ctx *FieldCallFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDeepNextFilter(ctx *DeepNextFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitConfiggedDeepNextFilter(ctx *ConfiggedDeepNextFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNamedParam(ctx *NamedParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitEmptyParam(ctx *EmptyParamContext) interface{} {
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
