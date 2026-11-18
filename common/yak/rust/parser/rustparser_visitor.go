// Code generated from ./RustParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package rustparser // RustParser
import "github.com/yaklang/antlr/v4"

// A complete Visitor for a parse tree produced by RustParser.
type RustParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by RustParser#crate.
	VisitCrate(ctx *CrateContext) interface{}

	// Visit a parse tree produced by RustParser#macroInvocation.
	VisitMacroInvocation(ctx *MacroInvocationContext) interface{}

	// Visit a parse tree produced by RustParser#delimTokenTree.
	VisitDelimTokenTree(ctx *DelimTokenTreeContext) interface{}

	// Visit a parse tree produced by RustParser#tokenTree.
	VisitTokenTree(ctx *TokenTreeContext) interface{}

	// Visit a parse tree produced by RustParser#tokenTreeToken.
	VisitTokenTreeToken(ctx *TokenTreeTokenContext) interface{}

	// Visit a parse tree produced by RustParser#macroInvocationSemi.
	VisitMacroInvocationSemi(ctx *MacroInvocationSemiContext) interface{}

	// Visit a parse tree produced by RustParser#macroRulesDefinition.
	VisitMacroRulesDefinition(ctx *MacroRulesDefinitionContext) interface{}

	// Visit a parse tree produced by RustParser#macroRulesDef.
	VisitMacroRulesDef(ctx *MacroRulesDefContext) interface{}

	// Visit a parse tree produced by RustParser#macroRules.
	VisitMacroRules(ctx *MacroRulesContext) interface{}

	// Visit a parse tree produced by RustParser#macroRule.
	VisitMacroRule(ctx *MacroRuleContext) interface{}

	// Visit a parse tree produced by RustParser#macroMatcher.
	VisitMacroMatcher(ctx *MacroMatcherContext) interface{}

	// Visit a parse tree produced by RustParser#macroMatch.
	VisitMacroMatch(ctx *MacroMatchContext) interface{}

	// Visit a parse tree produced by RustParser#macroMatchToken.
	VisitMacroMatchToken(ctx *MacroMatchTokenContext) interface{}

	// Visit a parse tree produced by RustParser#macroFragSpec.
	VisitMacroFragSpec(ctx *MacroFragSpecContext) interface{}

	// Visit a parse tree produced by RustParser#macroRepSep.
	VisitMacroRepSep(ctx *MacroRepSepContext) interface{}

	// Visit a parse tree produced by RustParser#macroRepOp.
	VisitMacroRepOp(ctx *MacroRepOpContext) interface{}

	// Visit a parse tree produced by RustParser#macroTranscriber.
	VisitMacroTranscriber(ctx *MacroTranscriberContext) interface{}

	// Visit a parse tree produced by RustParser#item.
	VisitItem(ctx *ItemContext) interface{}

	// Visit a parse tree produced by RustParser#visItem.
	VisitVisItem(ctx *VisItemContext) interface{}

	// Visit a parse tree produced by RustParser#macroItem.
	VisitMacroItem(ctx *MacroItemContext) interface{}

	// Visit a parse tree produced by RustParser#module.
	VisitModule(ctx *ModuleContext) interface{}

	// Visit a parse tree produced by RustParser#externCrate.
	VisitExternCrate(ctx *ExternCrateContext) interface{}

	// Visit a parse tree produced by RustParser#crateRef.
	VisitCrateRef(ctx *CrateRefContext) interface{}

	// Visit a parse tree produced by RustParser#asClause.
	VisitAsClause(ctx *AsClauseContext) interface{}

	// Visit a parse tree produced by RustParser#useDeclaration.
	VisitUseDeclaration(ctx *UseDeclarationContext) interface{}

	// Visit a parse tree produced by RustParser#useTree.
	VisitUseTree(ctx *UseTreeContext) interface{}

	// Visit a parse tree produced by RustParser#function_.
	VisitFunction_(ctx *Function_Context) interface{}

	// Visit a parse tree produced by RustParser#functionQualifiers.
	VisitFunctionQualifiers(ctx *FunctionQualifiersContext) interface{}

	// Visit a parse tree produced by RustParser#abi.
	VisitAbi(ctx *AbiContext) interface{}

	// Visit a parse tree produced by RustParser#functionParameters.
	VisitFunctionParameters(ctx *FunctionParametersContext) interface{}

	// Visit a parse tree produced by RustParser#selfParam.
	VisitSelfParam(ctx *SelfParamContext) interface{}

	// Visit a parse tree produced by RustParser#shorthandSelf.
	VisitShorthandSelf(ctx *ShorthandSelfContext) interface{}

	// Visit a parse tree produced by RustParser#typedSelf.
	VisitTypedSelf(ctx *TypedSelfContext) interface{}

	// Visit a parse tree produced by RustParser#functionParam.
	VisitFunctionParam(ctx *FunctionParamContext) interface{}

	// Visit a parse tree produced by RustParser#functionParamPattern.
	VisitFunctionParamPattern(ctx *FunctionParamPatternContext) interface{}

	// Visit a parse tree produced by RustParser#functionReturnType.
	VisitFunctionReturnType(ctx *FunctionReturnTypeContext) interface{}

	// Visit a parse tree produced by RustParser#typeAlias.
	VisitTypeAlias(ctx *TypeAliasContext) interface{}

	// Visit a parse tree produced by RustParser#struct_.
	VisitStruct_(ctx *Struct_Context) interface{}

	// Visit a parse tree produced by RustParser#structStruct.
	VisitStructStruct(ctx *StructStructContext) interface{}

	// Visit a parse tree produced by RustParser#tupleStruct.
	VisitTupleStruct(ctx *TupleStructContext) interface{}

	// Visit a parse tree produced by RustParser#structFields.
	VisitStructFields(ctx *StructFieldsContext) interface{}

	// Visit a parse tree produced by RustParser#structField.
	VisitStructField(ctx *StructFieldContext) interface{}

	// Visit a parse tree produced by RustParser#tupleFields.
	VisitTupleFields(ctx *TupleFieldsContext) interface{}

	// Visit a parse tree produced by RustParser#tupleField.
	VisitTupleField(ctx *TupleFieldContext) interface{}

	// Visit a parse tree produced by RustParser#enumeration.
	VisitEnumeration(ctx *EnumerationContext) interface{}

	// Visit a parse tree produced by RustParser#enumItems.
	VisitEnumItems(ctx *EnumItemsContext) interface{}

	// Visit a parse tree produced by RustParser#enumItem.
	VisitEnumItem(ctx *EnumItemContext) interface{}

	// Visit a parse tree produced by RustParser#enumItemTuple.
	VisitEnumItemTuple(ctx *EnumItemTupleContext) interface{}

	// Visit a parse tree produced by RustParser#enumItemStruct.
	VisitEnumItemStruct(ctx *EnumItemStructContext) interface{}

	// Visit a parse tree produced by RustParser#enumItemDiscriminant.
	VisitEnumItemDiscriminant(ctx *EnumItemDiscriminantContext) interface{}

	// Visit a parse tree produced by RustParser#union_.
	VisitUnion_(ctx *Union_Context) interface{}

	// Visit a parse tree produced by RustParser#constantItem.
	VisitConstantItem(ctx *ConstantItemContext) interface{}

	// Visit a parse tree produced by RustParser#staticItem.
	VisitStaticItem(ctx *StaticItemContext) interface{}

	// Visit a parse tree produced by RustParser#trait_.
	VisitTrait_(ctx *Trait_Context) interface{}

	// Visit a parse tree produced by RustParser#implementation.
	VisitImplementation(ctx *ImplementationContext) interface{}

	// Visit a parse tree produced by RustParser#inherentImpl.
	VisitInherentImpl(ctx *InherentImplContext) interface{}

	// Visit a parse tree produced by RustParser#traitImpl.
	VisitTraitImpl(ctx *TraitImplContext) interface{}

	// Visit a parse tree produced by RustParser#externBlock.
	VisitExternBlock(ctx *ExternBlockContext) interface{}

	// Visit a parse tree produced by RustParser#externalItem.
	VisitExternalItem(ctx *ExternalItemContext) interface{}

	// Visit a parse tree produced by RustParser#genericParams.
	VisitGenericParams(ctx *GenericParamsContext) interface{}

	// Visit a parse tree produced by RustParser#genericParam.
	VisitGenericParam(ctx *GenericParamContext) interface{}

	// Visit a parse tree produced by RustParser#lifetimeParam.
	VisitLifetimeParam(ctx *LifetimeParamContext) interface{}

	// Visit a parse tree produced by RustParser#typeParam.
	VisitTypeParam(ctx *TypeParamContext) interface{}

	// Visit a parse tree produced by RustParser#constParam.
	VisitConstParam(ctx *ConstParamContext) interface{}

	// Visit a parse tree produced by RustParser#whereClause.
	VisitWhereClause(ctx *WhereClauseContext) interface{}

	// Visit a parse tree produced by RustParser#whereClauseItem.
	VisitWhereClauseItem(ctx *WhereClauseItemContext) interface{}

	// Visit a parse tree produced by RustParser#lifetimeWhereClauseItem.
	VisitLifetimeWhereClauseItem(ctx *LifetimeWhereClauseItemContext) interface{}

	// Visit a parse tree produced by RustParser#typeBoundWhereClauseItem.
	VisitTypeBoundWhereClauseItem(ctx *TypeBoundWhereClauseItemContext) interface{}

	// Visit a parse tree produced by RustParser#forLifetimes.
	VisitForLifetimes(ctx *ForLifetimesContext) interface{}

	// Visit a parse tree produced by RustParser#associatedItem.
	VisitAssociatedItem(ctx *AssociatedItemContext) interface{}

	// Visit a parse tree produced by RustParser#innerAttribute.
	VisitInnerAttribute(ctx *InnerAttributeContext) interface{}

	// Visit a parse tree produced by RustParser#outerAttribute.
	VisitOuterAttribute(ctx *OuterAttributeContext) interface{}

	// Visit a parse tree produced by RustParser#attr.
	VisitAttr(ctx *AttrContext) interface{}

	// Visit a parse tree produced by RustParser#attrInput.
	VisitAttrInput(ctx *AttrInputContext) interface{}

	// Visit a parse tree produced by RustParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by RustParser#letStatement.
	VisitLetStatement(ctx *LetStatementContext) interface{}

	// Visit a parse tree produced by RustParser#expressionStatement.
	VisitExpressionStatement(ctx *ExpressionStatementContext) interface{}

	// Visit a parse tree produced by RustParser#TypeCastExpression.
	VisitTypeCastExpression(ctx *TypeCastExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#PathExpression_.
	VisitPathExpression_(ctx *PathExpression_Context) interface{}

	// Visit a parse tree produced by RustParser#TupleExpression.
	VisitTupleExpression(ctx *TupleExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#IndexExpression.
	VisitIndexExpression(ctx *IndexExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#RangeExpression.
	VisitRangeExpression(ctx *RangeExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#MacroInvocationAsExpression.
	VisitMacroInvocationAsExpression(ctx *MacroInvocationAsExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#ReturnExpression.
	VisitReturnExpression(ctx *ReturnExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#AwaitExpression.
	VisitAwaitExpression(ctx *AwaitExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#ErrorPropagationExpression.
	VisitErrorPropagationExpression(ctx *ErrorPropagationExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#ContinueExpression.
	VisitContinueExpression(ctx *ContinueExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#AssignmentExpression.
	VisitAssignmentExpression(ctx *AssignmentExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#MethodCallExpression.
	VisitMethodCallExpression(ctx *MethodCallExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#LiteralExpression_.
	VisitLiteralExpression_(ctx *LiteralExpression_Context) interface{}

	// Visit a parse tree produced by RustParser#StructExpression_.
	VisitStructExpression_(ctx *StructExpression_Context) interface{}

	// Visit a parse tree produced by RustParser#TupleIndexingExpression.
	VisitTupleIndexingExpression(ctx *TupleIndexingExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#NegationExpression.
	VisitNegationExpression(ctx *NegationExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#CallExpression.
	VisitCallExpression(ctx *CallExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#LazyBooleanExpression.
	VisitLazyBooleanExpression(ctx *LazyBooleanExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#DereferenceExpression.
	VisitDereferenceExpression(ctx *DereferenceExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#ExpressionWithBlock_.
	VisitExpressionWithBlock_(ctx *ExpressionWithBlock_Context) interface{}

	// Visit a parse tree produced by RustParser#GroupedExpression.
	VisitGroupedExpression(ctx *GroupedExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#BreakExpression.
	VisitBreakExpression(ctx *BreakExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#ArithmeticOrLogicalExpression.
	VisitArithmeticOrLogicalExpression(ctx *ArithmeticOrLogicalExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#FieldExpression.
	VisitFieldExpression(ctx *FieldExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#EnumerationVariantExpression_.
	VisitEnumerationVariantExpression_(ctx *EnumerationVariantExpression_Context) interface{}

	// Visit a parse tree produced by RustParser#ComparisonExpression.
	VisitComparisonExpression(ctx *ComparisonExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#AttributedExpression.
	VisitAttributedExpression(ctx *AttributedExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#BorrowExpression.
	VisitBorrowExpression(ctx *BorrowExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#CompoundAssignmentExpression.
	VisitCompoundAssignmentExpression(ctx *CompoundAssignmentExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#ClosureExpression_.
	VisitClosureExpression_(ctx *ClosureExpression_Context) interface{}

	// Visit a parse tree produced by RustParser#ArrayExpression.
	VisitArrayExpression(ctx *ArrayExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#comparisonOperator.
	VisitComparisonOperator(ctx *ComparisonOperatorContext) interface{}

	// Visit a parse tree produced by RustParser#compoundAssignOperator.
	VisitCompoundAssignOperator(ctx *CompoundAssignOperatorContext) interface{}

	// Visit a parse tree produced by RustParser#expressionWithBlock.
	VisitExpressionWithBlock(ctx *ExpressionWithBlockContext) interface{}

	// Visit a parse tree produced by RustParser#literalExpression.
	VisitLiteralExpression(ctx *LiteralExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#pathExpression.
	VisitPathExpression(ctx *PathExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#blockExpression.
	VisitBlockExpression(ctx *BlockExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#statements.
	VisitStatements(ctx *StatementsContext) interface{}

	// Visit a parse tree produced by RustParser#asyncBlockExpression.
	VisitAsyncBlockExpression(ctx *AsyncBlockExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#unsafeBlockExpression.
	VisitUnsafeBlockExpression(ctx *UnsafeBlockExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#arrayElements.
	VisitArrayElements(ctx *ArrayElementsContext) interface{}

	// Visit a parse tree produced by RustParser#tupleElements.
	VisitTupleElements(ctx *TupleElementsContext) interface{}

	// Visit a parse tree produced by RustParser#tupleIndex.
	VisitTupleIndex(ctx *TupleIndexContext) interface{}

	// Visit a parse tree produced by RustParser#structExpression.
	VisitStructExpression(ctx *StructExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#structExprStruct.
	VisitStructExprStruct(ctx *StructExprStructContext) interface{}

	// Visit a parse tree produced by RustParser#structExprFields.
	VisitStructExprFields(ctx *StructExprFieldsContext) interface{}

	// Visit a parse tree produced by RustParser#structExprField.
	VisitStructExprField(ctx *StructExprFieldContext) interface{}

	// Visit a parse tree produced by RustParser#structBase.
	VisitStructBase(ctx *StructBaseContext) interface{}

	// Visit a parse tree produced by RustParser#structExprTuple.
	VisitStructExprTuple(ctx *StructExprTupleContext) interface{}

	// Visit a parse tree produced by RustParser#structExprUnit.
	VisitStructExprUnit(ctx *StructExprUnitContext) interface{}

	// Visit a parse tree produced by RustParser#enumerationVariantExpression.
	VisitEnumerationVariantExpression(ctx *EnumerationVariantExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#enumExprStruct.
	VisitEnumExprStruct(ctx *EnumExprStructContext) interface{}

	// Visit a parse tree produced by RustParser#enumExprFields.
	VisitEnumExprFields(ctx *EnumExprFieldsContext) interface{}

	// Visit a parse tree produced by RustParser#enumExprField.
	VisitEnumExprField(ctx *EnumExprFieldContext) interface{}

	// Visit a parse tree produced by RustParser#enumExprTuple.
	VisitEnumExprTuple(ctx *EnumExprTupleContext) interface{}

	// Visit a parse tree produced by RustParser#enumExprFieldless.
	VisitEnumExprFieldless(ctx *EnumExprFieldlessContext) interface{}

	// Visit a parse tree produced by RustParser#callParams.
	VisitCallParams(ctx *CallParamsContext) interface{}

	// Visit a parse tree produced by RustParser#closureExpression.
	VisitClosureExpression(ctx *ClosureExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#closureParameters.
	VisitClosureParameters(ctx *ClosureParametersContext) interface{}

	// Visit a parse tree produced by RustParser#closureParam.
	VisitClosureParam(ctx *ClosureParamContext) interface{}

	// Visit a parse tree produced by RustParser#loopExpression.
	VisitLoopExpression(ctx *LoopExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#infiniteLoopExpression.
	VisitInfiniteLoopExpression(ctx *InfiniteLoopExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#predicateLoopExpression.
	VisitPredicateLoopExpression(ctx *PredicateLoopExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#predicatePatternLoopExpression.
	VisitPredicatePatternLoopExpression(ctx *PredicatePatternLoopExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#iteratorLoopExpression.
	VisitIteratorLoopExpression(ctx *IteratorLoopExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#loopLabel.
	VisitLoopLabel(ctx *LoopLabelContext) interface{}

	// Visit a parse tree produced by RustParser#ifExpression.
	VisitIfExpression(ctx *IfExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#ifLetExpression.
	VisitIfLetExpression(ctx *IfLetExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#matchExpression.
	VisitMatchExpression(ctx *MatchExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#matchArms.
	VisitMatchArms(ctx *MatchArmsContext) interface{}

	// Visit a parse tree produced by RustParser#matchArmExpression.
	VisitMatchArmExpression(ctx *MatchArmExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#matchArm.
	VisitMatchArm(ctx *MatchArmContext) interface{}

	// Visit a parse tree produced by RustParser#matchArmGuard.
	VisitMatchArmGuard(ctx *MatchArmGuardContext) interface{}

	// Visit a parse tree produced by RustParser#pattern.
	VisitPattern(ctx *PatternContext) interface{}

	// Visit a parse tree produced by RustParser#patternNoTopAlt.
	VisitPatternNoTopAlt(ctx *PatternNoTopAltContext) interface{}

	// Visit a parse tree produced by RustParser#patternWithoutRange.
	VisitPatternWithoutRange(ctx *PatternWithoutRangeContext) interface{}

	// Visit a parse tree produced by RustParser#literalPattern.
	VisitLiteralPattern(ctx *LiteralPatternContext) interface{}

	// Visit a parse tree produced by RustParser#identifierPattern.
	VisitIdentifierPattern(ctx *IdentifierPatternContext) interface{}

	// Visit a parse tree produced by RustParser#wildcardPattern.
	VisitWildcardPattern(ctx *WildcardPatternContext) interface{}

	// Visit a parse tree produced by RustParser#restPattern.
	VisitRestPattern(ctx *RestPatternContext) interface{}

	// Visit a parse tree produced by RustParser#InclusiveRangePattern.
	VisitInclusiveRangePattern(ctx *InclusiveRangePatternContext) interface{}

	// Visit a parse tree produced by RustParser#HalfOpenRangePattern.
	VisitHalfOpenRangePattern(ctx *HalfOpenRangePatternContext) interface{}

	// Visit a parse tree produced by RustParser#ObsoleteRangePattern.
	VisitObsoleteRangePattern(ctx *ObsoleteRangePatternContext) interface{}

	// Visit a parse tree produced by RustParser#rangePatternBound.
	VisitRangePatternBound(ctx *RangePatternBoundContext) interface{}

	// Visit a parse tree produced by RustParser#referencePattern.
	VisitReferencePattern(ctx *ReferencePatternContext) interface{}

	// Visit a parse tree produced by RustParser#structPattern.
	VisitStructPattern(ctx *StructPatternContext) interface{}

	// Visit a parse tree produced by RustParser#structPatternElements.
	VisitStructPatternElements(ctx *StructPatternElementsContext) interface{}

	// Visit a parse tree produced by RustParser#structPatternFields.
	VisitStructPatternFields(ctx *StructPatternFieldsContext) interface{}

	// Visit a parse tree produced by RustParser#structPatternField.
	VisitStructPatternField(ctx *StructPatternFieldContext) interface{}

	// Visit a parse tree produced by RustParser#structPatternEtCetera.
	VisitStructPatternEtCetera(ctx *StructPatternEtCeteraContext) interface{}

	// Visit a parse tree produced by RustParser#tupleStructPattern.
	VisitTupleStructPattern(ctx *TupleStructPatternContext) interface{}

	// Visit a parse tree produced by RustParser#tupleStructItems.
	VisitTupleStructItems(ctx *TupleStructItemsContext) interface{}

	// Visit a parse tree produced by RustParser#tuplePattern.
	VisitTuplePattern(ctx *TuplePatternContext) interface{}

	// Visit a parse tree produced by RustParser#tuplePatternItems.
	VisitTuplePatternItems(ctx *TuplePatternItemsContext) interface{}

	// Visit a parse tree produced by RustParser#groupedPattern.
	VisitGroupedPattern(ctx *GroupedPatternContext) interface{}

	// Visit a parse tree produced by RustParser#slicePattern.
	VisitSlicePattern(ctx *SlicePatternContext) interface{}

	// Visit a parse tree produced by RustParser#slicePatternItems.
	VisitSlicePatternItems(ctx *SlicePatternItemsContext) interface{}

	// Visit a parse tree produced by RustParser#pathPattern.
	VisitPathPattern(ctx *PathPatternContext) interface{}

	// Visit a parse tree produced by RustParser#type_.
	VisitType_(ctx *Type_Context) interface{}

	// Visit a parse tree produced by RustParser#typeNoBounds.
	VisitTypeNoBounds(ctx *TypeNoBoundsContext) interface{}

	// Visit a parse tree produced by RustParser#parenthesizedType.
	VisitParenthesizedType(ctx *ParenthesizedTypeContext) interface{}

	// Visit a parse tree produced by RustParser#neverType.
	VisitNeverType(ctx *NeverTypeContext) interface{}

	// Visit a parse tree produced by RustParser#tupleType.
	VisitTupleType(ctx *TupleTypeContext) interface{}

	// Visit a parse tree produced by RustParser#arrayType.
	VisitArrayType(ctx *ArrayTypeContext) interface{}

	// Visit a parse tree produced by RustParser#sliceType.
	VisitSliceType(ctx *SliceTypeContext) interface{}

	// Visit a parse tree produced by RustParser#referenceType.
	VisitReferenceType(ctx *ReferenceTypeContext) interface{}

	// Visit a parse tree produced by RustParser#rawPointerType.
	VisitRawPointerType(ctx *RawPointerTypeContext) interface{}

	// Visit a parse tree produced by RustParser#bareFunctionType.
	VisitBareFunctionType(ctx *BareFunctionTypeContext) interface{}

	// Visit a parse tree produced by RustParser#functionTypeQualifiers.
	VisitFunctionTypeQualifiers(ctx *FunctionTypeQualifiersContext) interface{}

	// Visit a parse tree produced by RustParser#bareFunctionReturnType.
	VisitBareFunctionReturnType(ctx *BareFunctionReturnTypeContext) interface{}

	// Visit a parse tree produced by RustParser#functionParametersMaybeNamedVariadic.
	VisitFunctionParametersMaybeNamedVariadic(ctx *FunctionParametersMaybeNamedVariadicContext) interface{}

	// Visit a parse tree produced by RustParser#maybeNamedFunctionParameters.
	VisitMaybeNamedFunctionParameters(ctx *MaybeNamedFunctionParametersContext) interface{}

	// Visit a parse tree produced by RustParser#maybeNamedParam.
	VisitMaybeNamedParam(ctx *MaybeNamedParamContext) interface{}

	// Visit a parse tree produced by RustParser#maybeNamedFunctionParametersVariadic.
	VisitMaybeNamedFunctionParametersVariadic(ctx *MaybeNamedFunctionParametersVariadicContext) interface{}

	// Visit a parse tree produced by RustParser#traitObjectType.
	VisitTraitObjectType(ctx *TraitObjectTypeContext) interface{}

	// Visit a parse tree produced by RustParser#traitObjectTypeOneBound.
	VisitTraitObjectTypeOneBound(ctx *TraitObjectTypeOneBoundContext) interface{}

	// Visit a parse tree produced by RustParser#implTraitType.
	VisitImplTraitType(ctx *ImplTraitTypeContext) interface{}

	// Visit a parse tree produced by RustParser#implTraitTypeOneBound.
	VisitImplTraitTypeOneBound(ctx *ImplTraitTypeOneBoundContext) interface{}

	// Visit a parse tree produced by RustParser#inferredType.
	VisitInferredType(ctx *InferredTypeContext) interface{}

	// Visit a parse tree produced by RustParser#typeParamBounds.
	VisitTypeParamBounds(ctx *TypeParamBoundsContext) interface{}

	// Visit a parse tree produced by RustParser#typeParamBound.
	VisitTypeParamBound(ctx *TypeParamBoundContext) interface{}

	// Visit a parse tree produced by RustParser#traitBound.
	VisitTraitBound(ctx *TraitBoundContext) interface{}

	// Visit a parse tree produced by RustParser#lifetimeBounds.
	VisitLifetimeBounds(ctx *LifetimeBoundsContext) interface{}

	// Visit a parse tree produced by RustParser#lifetime.
	VisitLifetime(ctx *LifetimeContext) interface{}

	// Visit a parse tree produced by RustParser#simplePath.
	VisitSimplePath(ctx *SimplePathContext) interface{}

	// Visit a parse tree produced by RustParser#simplePathSegment.
	VisitSimplePathSegment(ctx *SimplePathSegmentContext) interface{}

	// Visit a parse tree produced by RustParser#pathInExpression.
	VisitPathInExpression(ctx *PathInExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#pathExprSegment.
	VisitPathExprSegment(ctx *PathExprSegmentContext) interface{}

	// Visit a parse tree produced by RustParser#pathIdentSegment.
	VisitPathIdentSegment(ctx *PathIdentSegmentContext) interface{}

	// Visit a parse tree produced by RustParser#genericArgs.
	VisitGenericArgs(ctx *GenericArgsContext) interface{}

	// Visit a parse tree produced by RustParser#genericArg.
	VisitGenericArg(ctx *GenericArgContext) interface{}

	// Visit a parse tree produced by RustParser#genericArgsConst.
	VisitGenericArgsConst(ctx *GenericArgsConstContext) interface{}

	// Visit a parse tree produced by RustParser#genericArgsLifetimes.
	VisitGenericArgsLifetimes(ctx *GenericArgsLifetimesContext) interface{}

	// Visit a parse tree produced by RustParser#genericArgsTypes.
	VisitGenericArgsTypes(ctx *GenericArgsTypesContext) interface{}

	// Visit a parse tree produced by RustParser#genericArgsBindings.
	VisitGenericArgsBindings(ctx *GenericArgsBindingsContext) interface{}

	// Visit a parse tree produced by RustParser#genericArgsBinding.
	VisitGenericArgsBinding(ctx *GenericArgsBindingContext) interface{}

	// Visit a parse tree produced by RustParser#qualifiedPathInExpression.
	VisitQualifiedPathInExpression(ctx *QualifiedPathInExpressionContext) interface{}

	// Visit a parse tree produced by RustParser#qualifiedPathType.
	VisitQualifiedPathType(ctx *QualifiedPathTypeContext) interface{}

	// Visit a parse tree produced by RustParser#qualifiedPathInType.
	VisitQualifiedPathInType(ctx *QualifiedPathInTypeContext) interface{}

	// Visit a parse tree produced by RustParser#typePath.
	VisitTypePath(ctx *TypePathContext) interface{}

	// Visit a parse tree produced by RustParser#typePathSegment.
	VisitTypePathSegment(ctx *TypePathSegmentContext) interface{}

	// Visit a parse tree produced by RustParser#typePathFn.
	VisitTypePathFn(ctx *TypePathFnContext) interface{}

	// Visit a parse tree produced by RustParser#typePathInputs.
	VisitTypePathInputs(ctx *TypePathInputsContext) interface{}

	// Visit a parse tree produced by RustParser#visibility.
	VisitVisibility(ctx *VisibilityContext) interface{}

	// Visit a parse tree produced by RustParser#identifier.
	VisitIdentifier(ctx *IdentifierContext) interface{}

	// Visit a parse tree produced by RustParser#keyword.
	VisitKeyword(ctx *KeywordContext) interface{}

	// Visit a parse tree produced by RustParser#macroIdentifierLikeToken.
	VisitMacroIdentifierLikeToken(ctx *MacroIdentifierLikeTokenContext) interface{}

	// Visit a parse tree produced by RustParser#macroLiteralToken.
	VisitMacroLiteralToken(ctx *MacroLiteralTokenContext) interface{}

	// Visit a parse tree produced by RustParser#macroPunctuationToken.
	VisitMacroPunctuationToken(ctx *MacroPunctuationTokenContext) interface{}

	// Visit a parse tree produced by RustParser#shl.
	VisitShl(ctx *ShlContext) interface{}

	// Visit a parse tree produced by RustParser#shr.
	VisitShr(ctx *ShrContext) interface{}
}
