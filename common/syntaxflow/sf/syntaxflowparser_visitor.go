// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package sf // SyntaxFlowParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by SyntaxFlowParser.
type SyntaxFlowParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by SyntaxFlowParser#flow.
	VisitFlow(ctx *FlowContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#statements.
	VisitStatements(ctx *StatementsContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Check.
	VisitCheck(ctx *CheckContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Description.
	VisitDescription(ctx *DescriptionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Alert.
	VisitAlert(ctx *AlertContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Filter.
	VisitFilter(ctx *FilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FileFilterContent.
	VisitFileFilterContent(ctx *FileFilterContentContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Command.
	VisitCommand(ctx *CommandContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#Empty.
	VisitEmpty(ctx *EmptyContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#fileFilterContentStatement.
	VisitFileFilterContentStatement(ctx *FileFilterContentStatementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#fileFilterContentInput.
	VisitFileFilterContentInput(ctx *FileFilterContentInputContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#fileFilterContentMethod.
	VisitFileFilterContentMethod(ctx *FileFilterContentMethodContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#fileFilterContentMethodParam.
	VisitFileFilterContentMethodParam(ctx *FileFilterContentMethodParamContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#fileFilterContentMethodParamItem.
	VisitFileFilterContentMethodParamItem(ctx *FileFilterContentMethodParamItemContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#fileFilterContentMethodParamKey.
	VisitFileFilterContentMethodParamKey(ctx *FileFilterContentMethodParamKeyContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#fileFilterContentMethodParamValue.
	VisitFileFilterContentMethodParamValue(ctx *FileFilterContentMethodParamValueContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#fileName.
	VisitFileName(ctx *FileNameContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#RefFilterExpr.
	VisitRefFilterExpr(ctx *RefFilterExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#PureFilterExpr.
	VisitPureFilterExpr(ctx *PureFilterExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#comment.
	VisitComment(ctx *CommentContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#eos.
	VisitEos(ctx *EosContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#line.
	VisitLine(ctx *LineContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#lines.
	VisitLines(ctx *LinesContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionStatement.
	VisitDescriptionStatement(ctx *DescriptionStatementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionItems.
	VisitDescriptionItems(ctx *DescriptionItemsContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionItem.
	VisitDescriptionItem(ctx *DescriptionItemContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionSep.
	VisitDescriptionSep(ctx *DescriptionSepContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#descriptionItemValue.
	VisitDescriptionItemValue(ctx *DescriptionItemValueContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#crlfHereDoc.
	VisitCrlfHereDoc(ctx *CrlfHereDocContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#lfHereDoc.
	VisitLfHereDoc(ctx *LfHereDocContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#crlfText.
	VisitCrlfText(ctx *CrlfTextContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#lfText.
	VisitLfText(ctx *LfTextContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#hereDoc.
	VisitHereDoc(ctx *HereDocContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#alertStatement.
	VisitAlertStatement(ctx *AlertStatementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#checkStatement.
	VisitCheckStatement(ctx *CheckStatementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#thenExpr.
	VisitThenExpr(ctx *ThenExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#elseExpr.
	VisitElseExpr(ctx *ElseExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#refVariable.
	VisitRefVariable(ctx *RefVariableContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#ConstFilter.
	VisitConstFilter(ctx *ConstFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#NamedFilter.
	VisitNamedFilter(ctx *NamedFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#FieldCallFilter.
	VisitFieldCallFilter(ctx *FieldCallFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#NativeCallFilter.
	VisitNativeCallFilter(ctx *NativeCallFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#constSearchPrefix.
	VisitConstSearchPrefix(ctx *ConstSearchPrefixContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#First.
	VisitFirst(ctx *FirstContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#DeepChainFilter.
	VisitDeepChainFilter(ctx *DeepChainFilterContext) interface{}

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

	// Visit a parse tree produced by SyntaxFlowParser#MergeRefFilter.
	VisitMergeRefFilter(ctx *MergeRefFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#RemoveRefFilter.
	VisitRemoveRefFilter(ctx *RemoveRefFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#IntersectionRefFilter.
	VisitIntersectionRefFilter(ctx *IntersectionRefFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#VersionInFilter.
	VisitVersionInFilter(ctx *VersionInFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#filterExpr.
	VisitFilterExpr(ctx *FilterExprContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#nativeCall.
	VisitNativeCall(ctx *NativeCallContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#useNativeCall.
	VisitUseNativeCall(ctx *UseNativeCallContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#useDefCalcParams.
	VisitUseDefCalcParams(ctx *UseDefCalcParamsContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#nativeCallActualParams.
	VisitNativeCallActualParams(ctx *NativeCallActualParamsContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#nativeCallActualParam.
	VisitNativeCallActualParam(ctx *NativeCallActualParamContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#nativeCallActualParamKey.
	VisitNativeCallActualParamKey(ctx *NativeCallActualParamKeyContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#nativeCallActualParamValue.
	VisitNativeCallActualParamValue(ctx *NativeCallActualParamValueContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#AllParam.
	VisitAllParam(ctx *AllParamContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#EveryParam.
	VisitEveryParam(ctx *EveryParamContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#actualParamFilter.
	VisitActualParamFilter(ctx *ActualParamFilterContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#singleParam.
	VisitSingleParam(ctx *SingleParamContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#config.
	VisitConfig(ctx *ConfigContext) interface{}

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

	// Visit a parse tree produced by SyntaxFlowParser#VersionInCondition.
	VisitVersionInCondition(ctx *VersionInConditionContext) interface{}

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

	// Visit a parse tree produced by SyntaxFlowParser#versionInExpression.
	VisitVersionInExpression(ctx *VersionInExpressionContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#versionInterval.
	VisitVersionInterval(ctx *VersionIntervalContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#vstart.
	VisitVstart(ctx *VstartContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#vend.
	VisitVend(ctx *VendContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#versionBlockElement.
	VisitVersionBlockElement(ctx *VersionBlockElementContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#versionSuffix.
	VisitVersionSuffix(ctx *VersionSuffixContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#versionBlock.
	VisitVersionBlock(ctx *VersionBlockContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#versionString.
	VisitVersionString(ctx *VersionStringContext) interface{}

	// Visit a parse tree produced by SyntaxFlowParser#opcodesCondition.
	VisitOpcodesCondition(ctx *OpcodesConditionContext) interface{}

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
