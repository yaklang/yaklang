// Code generated from ./RustParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package rustparser // RustParser
import "github.com/yaklang/antlr/v4"

type BaseRustParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseRustParserVisitor) VisitCrate(ctx *CrateContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroInvocation(ctx *MacroInvocationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitDelimTokenTree(ctx *DelimTokenTreeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTokenTree(ctx *TokenTreeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTokenTreeToken(ctx *TokenTreeTokenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroInvocationSemi(ctx *MacroInvocationSemiContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroRulesDefinition(ctx *MacroRulesDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroRulesDef(ctx *MacroRulesDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroRules(ctx *MacroRulesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroRule(ctx *MacroRuleContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroMatcher(ctx *MacroMatcherContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroMatch(ctx *MacroMatchContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroMatchToken(ctx *MacroMatchTokenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroFragSpec(ctx *MacroFragSpecContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroRepSep(ctx *MacroRepSepContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroRepOp(ctx *MacroRepOpContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroTranscriber(ctx *MacroTranscriberContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitItem(ctx *ItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitVisItem(ctx *VisItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroItem(ctx *MacroItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitModule(ctx *ModuleContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitExternCrate(ctx *ExternCrateContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitCrateRef(ctx *CrateRefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitAsClause(ctx *AsClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitUseDeclaration(ctx *UseDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitUseTree(ctx *UseTreeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitFunction_(ctx *Function_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitFunctionQualifiers(ctx *FunctionQualifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitAbi(ctx *AbiContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitFunctionParameters(ctx *FunctionParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitSelfParam(ctx *SelfParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitShorthandSelf(ctx *ShorthandSelfContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypedSelf(ctx *TypedSelfContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitFunctionParam(ctx *FunctionParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitFunctionParamPattern(ctx *FunctionParamPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitFunctionReturnType(ctx *FunctionReturnTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypeAlias(ctx *TypeAliasContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStruct_(ctx *Struct_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructStruct(ctx *StructStructContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTupleStruct(ctx *TupleStructContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructFields(ctx *StructFieldsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructField(ctx *StructFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTupleFields(ctx *TupleFieldsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTupleField(ctx *TupleFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumeration(ctx *EnumerationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumItems(ctx *EnumItemsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumItem(ctx *EnumItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumItemTuple(ctx *EnumItemTupleContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumItemStruct(ctx *EnumItemStructContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumItemDiscriminant(ctx *EnumItemDiscriminantContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitUnion_(ctx *Union_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitConstantItem(ctx *ConstantItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStaticItem(ctx *StaticItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTrait_(ctx *Trait_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitImplementation(ctx *ImplementationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitInherentImpl(ctx *InherentImplContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTraitImpl(ctx *TraitImplContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitExternBlock(ctx *ExternBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitExternalItem(ctx *ExternalItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGenericParams(ctx *GenericParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGenericParam(ctx *GenericParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLifetimeParam(ctx *LifetimeParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypeParam(ctx *TypeParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitConstParam(ctx *ConstParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitWhereClause(ctx *WhereClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitWhereClauseItem(ctx *WhereClauseItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLifetimeWhereClauseItem(ctx *LifetimeWhereClauseItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypeBoundWhereClauseItem(ctx *TypeBoundWhereClauseItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitForLifetimes(ctx *ForLifetimesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitAssociatedItem(ctx *AssociatedItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitInnerAttribute(ctx *InnerAttributeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitOuterAttribute(ctx *OuterAttributeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitAttr(ctx *AttrContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitAttrInput(ctx *AttrInputContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLetStatement(ctx *LetStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitExpressionStatement(ctx *ExpressionStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypeCastExpression(ctx *TypeCastExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPathExpression_(ctx *PathExpression_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTupleExpression(ctx *TupleExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitIndexExpression(ctx *IndexExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitRangeExpression(ctx *RangeExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroInvocationAsExpression(ctx *MacroInvocationAsExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitReturnExpression(ctx *ReturnExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitAwaitExpression(ctx *AwaitExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitErrorPropagationExpression(ctx *ErrorPropagationExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitContinueExpression(ctx *ContinueExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitAssignmentExpression(ctx *AssignmentExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMethodCallExpression(ctx *MethodCallExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLiteralExpression_(ctx *LiteralExpression_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructExpression_(ctx *StructExpression_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTupleIndexingExpression(ctx *TupleIndexingExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitNegationExpression(ctx *NegationExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitCallExpression(ctx *CallExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLazyBooleanExpression(ctx *LazyBooleanExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitDereferenceExpression(ctx *DereferenceExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitExpressionWithBlock_(ctx *ExpressionWithBlock_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGroupedExpression(ctx *GroupedExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitBreakExpression(ctx *BreakExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitArithmeticOrLogicalExpression(ctx *ArithmeticOrLogicalExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitFieldExpression(ctx *FieldExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumerationVariantExpression_(ctx *EnumerationVariantExpression_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitComparisonExpression(ctx *ComparisonExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitAttributedExpression(ctx *AttributedExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitBorrowExpression(ctx *BorrowExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitCompoundAssignmentExpression(ctx *CompoundAssignmentExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitClosureExpression_(ctx *ClosureExpression_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitArrayExpression(ctx *ArrayExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitComparisonOperator(ctx *ComparisonOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitCompoundAssignOperator(ctx *CompoundAssignOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitExpressionWithBlock(ctx *ExpressionWithBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLiteralExpression(ctx *LiteralExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPathExpression(ctx *PathExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitBlockExpression(ctx *BlockExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStatements(ctx *StatementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitAsyncBlockExpression(ctx *AsyncBlockExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitUnsafeBlockExpression(ctx *UnsafeBlockExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitArrayElements(ctx *ArrayElementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTupleElements(ctx *TupleElementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTupleIndex(ctx *TupleIndexContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructExpression(ctx *StructExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructExprStruct(ctx *StructExprStructContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructExprFields(ctx *StructExprFieldsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructExprField(ctx *StructExprFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructBase(ctx *StructBaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructExprTuple(ctx *StructExprTupleContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructExprUnit(ctx *StructExprUnitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumerationVariantExpression(ctx *EnumerationVariantExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumExprStruct(ctx *EnumExprStructContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumExprFields(ctx *EnumExprFieldsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumExprField(ctx *EnumExprFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumExprTuple(ctx *EnumExprTupleContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitEnumExprFieldless(ctx *EnumExprFieldlessContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitCallParams(ctx *CallParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitClosureExpression(ctx *ClosureExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitClosureParameters(ctx *ClosureParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitClosureParam(ctx *ClosureParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLoopExpression(ctx *LoopExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitInfiniteLoopExpression(ctx *InfiniteLoopExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPredicateLoopExpression(ctx *PredicateLoopExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPredicatePatternLoopExpression(ctx *PredicatePatternLoopExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitIteratorLoopExpression(ctx *IteratorLoopExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLoopLabel(ctx *LoopLabelContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitIfExpression(ctx *IfExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitIfLetExpression(ctx *IfLetExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMatchExpression(ctx *MatchExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMatchArms(ctx *MatchArmsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMatchArmExpression(ctx *MatchArmExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMatchArm(ctx *MatchArmContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMatchArmGuard(ctx *MatchArmGuardContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPattern(ctx *PatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPatternNoTopAlt(ctx *PatternNoTopAltContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPatternWithoutRange(ctx *PatternWithoutRangeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLiteralPattern(ctx *LiteralPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitIdentifierPattern(ctx *IdentifierPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitWildcardPattern(ctx *WildcardPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitRestPattern(ctx *RestPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitInclusiveRangePattern(ctx *InclusiveRangePatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitHalfOpenRangePattern(ctx *HalfOpenRangePatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitObsoleteRangePattern(ctx *ObsoleteRangePatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitRangePatternBound(ctx *RangePatternBoundContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitReferencePattern(ctx *ReferencePatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructPattern(ctx *StructPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructPatternElements(ctx *StructPatternElementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructPatternFields(ctx *StructPatternFieldsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructPatternField(ctx *StructPatternFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitStructPatternEtCetera(ctx *StructPatternEtCeteraContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTupleStructPattern(ctx *TupleStructPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTupleStructItems(ctx *TupleStructItemsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTuplePattern(ctx *TuplePatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTuplePatternItems(ctx *TuplePatternItemsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGroupedPattern(ctx *GroupedPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitSlicePattern(ctx *SlicePatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitSlicePatternItems(ctx *SlicePatternItemsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPathPattern(ctx *PathPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitType_(ctx *Type_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypeNoBounds(ctx *TypeNoBoundsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitParenthesizedType(ctx *ParenthesizedTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitNeverType(ctx *NeverTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTupleType(ctx *TupleTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitArrayType(ctx *ArrayTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitSliceType(ctx *SliceTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitReferenceType(ctx *ReferenceTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitRawPointerType(ctx *RawPointerTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitBareFunctionType(ctx *BareFunctionTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitFunctionTypeQualifiers(ctx *FunctionTypeQualifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitBareFunctionReturnType(ctx *BareFunctionReturnTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitFunctionParametersMaybeNamedVariadic(ctx *FunctionParametersMaybeNamedVariadicContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMaybeNamedFunctionParameters(ctx *MaybeNamedFunctionParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMaybeNamedParam(ctx *MaybeNamedParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMaybeNamedFunctionParametersVariadic(ctx *MaybeNamedFunctionParametersVariadicContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTraitObjectType(ctx *TraitObjectTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTraitObjectTypeOneBound(ctx *TraitObjectTypeOneBoundContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitImplTraitType(ctx *ImplTraitTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitImplTraitTypeOneBound(ctx *ImplTraitTypeOneBoundContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitInferredType(ctx *InferredTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypeParamBounds(ctx *TypeParamBoundsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypeParamBound(ctx *TypeParamBoundContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTraitBound(ctx *TraitBoundContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLifetimeBounds(ctx *LifetimeBoundsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitLifetime(ctx *LifetimeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitSimplePath(ctx *SimplePathContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitSimplePathSegment(ctx *SimplePathSegmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPathInExpression(ctx *PathInExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPathExprSegment(ctx *PathExprSegmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitPathIdentSegment(ctx *PathIdentSegmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGenericArgs(ctx *GenericArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGenericArg(ctx *GenericArgContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGenericArgsConst(ctx *GenericArgsConstContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGenericArgsLifetimes(ctx *GenericArgsLifetimesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGenericArgsTypes(ctx *GenericArgsTypesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGenericArgsBindings(ctx *GenericArgsBindingsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitGenericArgsBinding(ctx *GenericArgsBindingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitQualifiedPathInExpression(ctx *QualifiedPathInExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitQualifiedPathType(ctx *QualifiedPathTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitQualifiedPathInType(ctx *QualifiedPathInTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypePath(ctx *TypePathContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypePathSegment(ctx *TypePathSegmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypePathFn(ctx *TypePathFnContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitTypePathInputs(ctx *TypePathInputsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitVisibility(ctx *VisibilityContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitIdentifier(ctx *IdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitKeyword(ctx *KeywordContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroIdentifierLikeToken(ctx *MacroIdentifierLikeTokenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroLiteralToken(ctx *MacroLiteralTokenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitMacroPunctuationToken(ctx *MacroPunctuationTokenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitShl(ctx *ShlContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseRustParserVisitor) VisitShr(ctx *ShrContext) interface{} {
	return v.VisitChildren(ctx)
}
