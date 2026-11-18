// Code generated from ./KotlinParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package kotlinparser // KotlinParser
import "github.com/yaklang/antlr/v4"

type BaseKotlinParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseKotlinParserVisitor) VisitKotlinFile(ctx *KotlinFileContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitScript(ctx *ScriptContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitFileAnnotation(ctx *FileAnnotationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPackageHeader(ctx *PackageHeaderContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitImportList(ctx *ImportListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitImportHeader(ctx *ImportHeaderContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitImportAlias(ctx *ImportAliasContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTopLevelObject(ctx *TopLevelObjectContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitClassDeclaration(ctx *ClassDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPrimaryConstructor(ctx *PrimaryConstructorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitClassParameters(ctx *ClassParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitClassParameter(ctx *ClassParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitDelegationSpecifiers(ctx *DelegationSpecifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAnnotatedDelegationSpecifier(ctx *AnnotatedDelegationSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitDelegationSpecifier(ctx *DelegationSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitConstructorInvocation(ctx *ConstructorInvocationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitExplicitDelegation(ctx *ExplicitDelegationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitClassBody(ctx *ClassBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitClassMemberDeclarations(ctx *ClassMemberDeclarationsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitClassMemberDeclaration(ctx *ClassMemberDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAnonymousInitializer(ctx *AnonymousInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitSecondaryConstructor(ctx *SecondaryConstructorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitConstructorDelegationCall(ctx *ConstructorDelegationCallContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitEnumClassBody(ctx *EnumClassBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitEnumEntries(ctx *EnumEntriesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitEnumEntry(ctx *EnumEntryContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitFunctionDeclaration(ctx *FunctionDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitFunctionValueParameters(ctx *FunctionValueParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitFunctionValueParameter(ctx *FunctionValueParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitParameter(ctx *ParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitSetterParameter(ctx *SetterParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitFunctionBody(ctx *FunctionBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitObjectDeclaration(ctx *ObjectDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitCompanionObject(ctx *CompanionObjectContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPropertyDeclaration(ctx *PropertyDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitMultiVariableDeclaration(ctx *MultiVariableDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitVariableDeclaration(ctx *VariableDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPropertyDelegate(ctx *PropertyDelegateContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitGetter(ctx *GetterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitSetter(ctx *SetterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeAlias(ctx *TypeAliasContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeParameters(ctx *TypeParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeParameter(ctx *TypeParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeParameterModifiers(ctx *TypeParameterModifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeParameterModifier(ctx *TypeParameterModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitType_(ctx *Type_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeModifiers(ctx *TypeModifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeModifier(ctx *TypeModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitParenthesizedType(ctx *ParenthesizedTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitNullableType(ctx *NullableTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeReference(ctx *TypeReferenceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitFunctionType(ctx *FunctionTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitReceiverType(ctx *ReceiverTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitUserType(ctx *UserTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitParenthesizedUserType(ctx *ParenthesizedUserTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitSimpleUserType(ctx *SimpleUserTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitFunctionTypeParameters(ctx *FunctionTypeParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeConstraints(ctx *TypeConstraintsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeConstraint(ctx *TypeConstraintContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitBlock(ctx *BlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitStatements(ctx *StatementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitDeclaration(ctx *DeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAssignment(ctx *AssignmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitExpression(ctx *ExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitDisjunction(ctx *DisjunctionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitConjunction(ctx *ConjunctionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitEquality(ctx *EqualityContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitComparison(ctx *ComparisonContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitInfixOperation(ctx *InfixOperationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitElvisExpression(ctx *ElvisExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitInfixFunctionCall(ctx *InfixFunctionCallContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitRangeExpression(ctx *RangeExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAsExpression(ctx *AsExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPrefixUnaryExpression(ctx *PrefixUnaryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitUnaryPrefix(ctx *UnaryPrefixContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPostfixUnaryExpression(ctx *PostfixUnaryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPostfixUnarySuffix(ctx *PostfixUnarySuffixContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitDirectlyAssignableExpression(ctx *DirectlyAssignableExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAssignableExpression(ctx *AssignableExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAssignableSuffix(ctx *AssignableSuffixContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitIndexingSuffix(ctx *IndexingSuffixContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitNavigationSuffix(ctx *NavigationSuffixContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitCallSuffix(ctx *CallSuffixContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAnnotatedLambda(ctx *AnnotatedLambdaContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitValueArguments(ctx *ValueArgumentsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeArguments(ctx *TypeArgumentsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeProjection(ctx *TypeProjectionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeProjectionModifiers(ctx *TypeProjectionModifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeProjectionModifier(ctx *TypeProjectionModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitValueArgument(ctx *ValueArgumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPrimaryExpression(ctx *PrimaryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitParenthesizedExpression(ctx *ParenthesizedExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitCollectionLiteral(ctx *CollectionLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitLiteralConstant(ctx *LiteralConstantContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitStringLiteral(ctx *StringLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitLineStringLiteral(ctx *LineStringLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitMultiLineStringLiteral(ctx *MultiLineStringLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitLineStringContent(ctx *LineStringContentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitLineStringExpression(ctx *LineStringExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitMultiLineStringContent(ctx *MultiLineStringContentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitMultiLineStringExpression(ctx *MultiLineStringExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitLambdaLiteral(ctx *LambdaLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitLambdaParameters(ctx *LambdaParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitLambdaParameter(ctx *LambdaParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAnonymousFunction(ctx *AnonymousFunctionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitFunctionLiteral(ctx *FunctionLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitObjectLiteral(ctx *ObjectLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitThisExpression(ctx *ThisExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitSuperExpression(ctx *SuperExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitControlStructureBody(ctx *ControlStructureBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitIfExpression(ctx *IfExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitWhenExpression(ctx *WhenExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitWhenSubject(ctx *WhenSubjectContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitWhenEntry(ctx *WhenEntryContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitWhenCondition(ctx *WhenConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitRangeTest(ctx *RangeTestContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTypeTest(ctx *TypeTestContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitTryExpression(ctx *TryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitCatchBlock(ctx *CatchBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitFinallyBlock(ctx *FinallyBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitLoopStatement(ctx *LoopStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitForStatement(ctx *ForStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitWhileStatement(ctx *WhileStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitDoWhileStatement(ctx *DoWhileStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitJumpExpression(ctx *JumpExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitCallableReference(ctx *CallableReferenceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAssignmentAndOperator(ctx *AssignmentAndOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitEqualityOperator(ctx *EqualityOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitComparisonOperator(ctx *ComparisonOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitInOperator(ctx *InOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitIsOperator(ctx *IsOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAdditiveOperator(ctx *AdditiveOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitMultiplicativeOperator(ctx *MultiplicativeOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAsOperator(ctx *AsOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPrefixUnaryOperator(ctx *PrefixUnaryOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPostfixUnaryOperator(ctx *PostfixUnaryOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitMemberAccessOperator(ctx *MemberAccessOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitModifiers(ctx *ModifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitModifier(ctx *ModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitClassModifier(ctx *ClassModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitMemberModifier(ctx *MemberModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitVisibilityModifier(ctx *VisibilityModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitVarianceModifier(ctx *VarianceModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitFunctionModifier(ctx *FunctionModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPropertyModifier(ctx *PropertyModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitInheritanceModifier(ctx *InheritanceModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitParameterModifier(ctx *ParameterModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitReificationModifier(ctx *ReificationModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitPlatformModifier(ctx *PlatformModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitLabel(ctx *LabelContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAnnotation(ctx *AnnotationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitSingleAnnotation(ctx *SingleAnnotationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitMultiAnnotation(ctx *MultiAnnotationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitAnnotationUseSiteTarget(ctx *AnnotationUseSiteTargetContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitUnescapedAnnotation(ctx *UnescapedAnnotationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitSimpleIdentifier(ctx *SimpleIdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitIdentifier(ctx *IdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitShebangLine(ctx *ShebangLineContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitQuest(ctx *QuestContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitElvis(ctx *ElvisContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitSafeNav(ctx *SafeNavContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitExcl(ctx *ExclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitSemi(ctx *SemiContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseKotlinParserVisitor) VisitSemis(ctx *SemisContext) interface{} {
	return v.VisitChildren(ctx)
}
