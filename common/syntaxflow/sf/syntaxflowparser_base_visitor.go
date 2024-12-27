// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package sf // SyntaxFlowParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type BaseSyntaxFlowParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseSyntaxFlowParserVisitor) VisitFlow(ctx *FlowContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitStatements(ctx *StatementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitCheck(ctx *CheckContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitDescription(ctx *DescriptionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitAlert(ctx *AlertContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFilter(ctx *FilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFileFilterContent(ctx *FileFilterContentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitCommand(ctx *CommandContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitEmpty(ctx *EmptyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFileFilterContentStatement(ctx *FileFilterContentStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFileFilterContentInput(ctx *FileFilterContentInputContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFileFilterContentMethod(ctx *FileFilterContentMethodContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFileFilterContentMethodParam(ctx *FileFilterContentMethodParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFileFilterContentMethodParamItem(ctx *FileFilterContentMethodParamItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFileFilterContentMethodParamKey(ctx *FileFilterContentMethodParamKeyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFileFilterContentMethodParamValue(ctx *FileFilterContentMethodParamValueContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFileName(ctx *FileNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitRefFilterExpr(ctx *RefFilterExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitPureFilterExpr(ctx *PureFilterExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitComment(ctx *CommentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitEos(ctx *EosContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitLine(ctx *LineContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitLines(ctx *LinesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitDescriptionStatement(ctx *DescriptionStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitDescriptionItems(ctx *DescriptionItemsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitDescriptionItem(ctx *DescriptionItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitDescriptionSep(ctx *DescriptionSepContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitDescriptionItemValue(ctx *DescriptionItemValueContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitCrlfHereDoc(ctx *CrlfHereDocContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitLfHereDoc(ctx *LfHereDocContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitCrlfText(ctx *CrlfTextContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitLfText(ctx *LfTextContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitHereDoc(ctx *HereDocContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitAlertStatement(ctx *AlertStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitCheckStatement(ctx *CheckStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitThenExpr(ctx *ThenExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitElseExpr(ctx *ElseExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitRefVariable(ctx *RefVariableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitConstFilter(ctx *ConstFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNamedFilter(ctx *NamedFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFieldCallFilter(ctx *FieldCallFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNativeCallFilter(ctx *NativeCallFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitConstSearchPrefix(ctx *ConstSearchPrefixContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFirst(ctx *FirstContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitDeepChainFilter(ctx *DeepChainFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFunctionCallFilter(ctx *FunctionCallFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFieldIndexFilter(ctx *FieldIndexFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitOptionalFilter(ctx *OptionalFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNextFilter(ctx *NextFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitDefFilter(ctx *DefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitDeepNextFilter(ctx *DeepNextFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitDeepNextConfigFilter(ctx *DeepNextConfigFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitTopDefFilter(ctx *TopDefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitTopDefConfigFilter(ctx *TopDefConfigFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitMergeRefFilter(ctx *MergeRefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitRemoveRefFilter(ctx *RemoveRefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitIntersectionRefFilter(ctx *IntersectionRefFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitVersionInFilter(ctx *VersionInFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFilterExpr(ctx *FilterExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNativeCall(ctx *NativeCallContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitUseNativeCall(ctx *UseNativeCallContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitUseDefCalcParams(ctx *UseDefCalcParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNativeCallActualParams(ctx *NativeCallActualParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNativeCallActualParam(ctx *NativeCallActualParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNativeCallActualParamKey(ctx *NativeCallActualParamKeyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNativeCallActualParamValue(ctx *NativeCallActualParamValueContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitAllParam(ctx *AllParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitEveryParam(ctx *EveryParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitActualParamFilter(ctx *ActualParamFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitSingleParam(ctx *SingleParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitConfig(ctx *ConfigContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitRecursiveConfigItem(ctx *RecursiveConfigItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitRecursiveConfigItemValue(ctx *RecursiveConfigItemValueContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitSliceCallItem(ctx *SliceCallItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNameFilter(ctx *NameFilterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFlat(ctx *FlatContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitBuildMap(ctx *BuildMapContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitStringLiteralWithoutStarGroup(ctx *StringLiteralWithoutStarGroupContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNegativeCondition(ctx *NegativeConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNotCondition(ctx *NotConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitParenCondition(ctx *ParenConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFilterCondition(ctx *FilterConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitOpcodeTypeCondition(ctx *OpcodeTypeConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitVersionInCondition(ctx *VersionInConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFilterExpressionOr(ctx *FilterExpressionOrContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFilterExpressionAnd(ctx *FilterExpressionAndContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFilterExpressionCompare(ctx *FilterExpressionCompareContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitFilterExpressionRegexpMatch(ctx *FilterExpressionRegexpMatchContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitStringContainAnyCondition(ctx *StringContainAnyConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitStringContainHaveCondition(ctx *StringContainHaveConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitVersionInExpression(ctx *VersionInExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitVersionInterval(ctx *VersionIntervalContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitVstart(ctx *VstartContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitVend(ctx *VendContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitVersionBlockElement(ctx *VersionBlockElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitVersionSuffix(ctx *VersionSuffixContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitVersionBlock(ctx *VersionBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitVersionString(ctx *VersionStringContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitOpcodesCondition(ctx *OpcodesConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitNumberLiteral(ctx *NumberLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitStringLiteral(ctx *StringLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitStringLiteralWithoutStar(ctx *StringLiteralWithoutStarContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitRegexpLiteral(ctx *RegexpLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitIdentifier(ctx *IdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitKeywords(ctx *KeywordsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitOpcodes(ctx *OpcodesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitTypes(ctx *TypesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseSyntaxFlowParserVisitor) VisitBoolLiteral(ctx *BoolLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}
