// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package sf // SyntaxFlow
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type BaseSyntaxFlowVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseSyntaxFlowVisitor) VisitFlow(ctx *FlowContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitStatements(ctx *StatementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitCheck(ctx *CheckContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDescription(ctx *DescriptionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitAlert(ctx *AlertContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilter(ctx *FilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFileFilterContent(ctx *FileFilterContentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitCommand(ctx *CommandContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitEmpty(ctx *EmptyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFileFilterContentStatement(ctx *FileFilterContentStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFileFilterContentInput(ctx *FileFilterContentInputContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFileFilterContentMethod(ctx *FileFilterContentMethodContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFileFilterContentMethodParam(ctx *FileFilterContentMethodParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFileFilterContentMethodParamItem(ctx *FileFilterContentMethodParamItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFileFilterContentMethodParamKey(ctx *FileFilterContentMethodParamKeyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFileFilterContentMethodParamValue(ctx *FileFilterContentMethodParamValueContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFileName(ctx *FileNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRefFilterExpr(ctx *RefFilterExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitPureFilterExpr(ctx *PureFilterExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitComment(ctx *CommentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitEos(ctx *EosContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitLine(ctx *LineContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitLines(ctx *LinesContext) interface{} {
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

func (v *BaseSyntaxFlowVisitor) VisitAlertStatement(ctx *AlertStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitCheckStatement(ctx *CheckStatementContext) interface{} {
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

func (v *BaseSyntaxFlowVisitor) VisitNamedFilter(ctx *NamedFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFieldCallFilter(ctx *FieldCallFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFirst(ctx *FirstContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDeepChainFilter(ctx *DeepChainFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFunctionCallFilter(ctx *FunctionCallFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFieldIndexFilter(ctx *FieldIndexFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitOptionalFilter(ctx *OptionalFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNextFilter(ctx *NextFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDefFilter(ctx *DefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDeepNextFilter(ctx *DeepNextFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitDeepNextConfigFilter(ctx *DeepNextConfigFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitTopDefFilter(ctx *TopDefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitTopDefConfigFilter(ctx *TopDefConfigFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNativeCallFilter(ctx *NativeCallFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitMergeRefFilter(ctx *MergeRefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRemoveRefFilter(ctx *RemoveRefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpr(ctx *FilterExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNativeCall(ctx *NativeCallContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitUseNativeCall(ctx *UseNativeCallContext) interface{} {
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

func (v *BaseSyntaxFlowVisitor) VisitConfig(ctx *ConfigContext) interface{} {
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

func (v *BaseSyntaxFlowVisitor) VisitStringLiteralWithoutStarGroup(ctx *StringLiteralWithoutStarGroupContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNegativeCondition(ctx *NegativeConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNotCondition(ctx *NotConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitParenCondition(ctx *ParenConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterCondition(ctx *FilterConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitOpcodeTypeCondition(ctx *OpcodeTypeConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitFilterExpressionOr(ctx *FilterExpressionOrContext) interface{} {
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

func (v *BaseSyntaxFlowVisitor) VisitStringContainAnyCondition(ctx *StringContainAnyConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitStringContainHaveCondition(ctx *StringContainHaveConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitNumberLiteral(ctx *NumberLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitStringLiteral(ctx *StringLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitStringLiteralWithoutStar(ctx *StringLiteralWithoutStarContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitRegexpLiteral(ctx *RegexpLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitIdentifier(ctx *IdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitKeywords(ctx *KeywordsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitOpcodes(ctx *OpcodesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitTypes(ctx *TypesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowVisitor) VisitBoolLiteral(ctx *BoolLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}
