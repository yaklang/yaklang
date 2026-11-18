// Code generated from ./KotlinParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package kotlinparser // KotlinParser
import "github.com/yaklang/antlr/v4"

// A complete Visitor for a parse tree produced by KotlinParser.
type KotlinParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by KotlinParser#kotlinFile.
	VisitKotlinFile(ctx *KotlinFileContext) interface{}

	// Visit a parse tree produced by KotlinParser#script.
	VisitScript(ctx *ScriptContext) interface{}

	// Visit a parse tree produced by KotlinParser#fileAnnotation.
	VisitFileAnnotation(ctx *FileAnnotationContext) interface{}

	// Visit a parse tree produced by KotlinParser#packageHeader.
	VisitPackageHeader(ctx *PackageHeaderContext) interface{}

	// Visit a parse tree produced by KotlinParser#importList.
	VisitImportList(ctx *ImportListContext) interface{}

	// Visit a parse tree produced by KotlinParser#importHeader.
	VisitImportHeader(ctx *ImportHeaderContext) interface{}

	// Visit a parse tree produced by KotlinParser#importAlias.
	VisitImportAlias(ctx *ImportAliasContext) interface{}

	// Visit a parse tree produced by KotlinParser#topLevelObject.
	VisitTopLevelObject(ctx *TopLevelObjectContext) interface{}

	// Visit a parse tree produced by KotlinParser#classDeclaration.
	VisitClassDeclaration(ctx *ClassDeclarationContext) interface{}

	// Visit a parse tree produced by KotlinParser#primaryConstructor.
	VisitPrimaryConstructor(ctx *PrimaryConstructorContext) interface{}

	// Visit a parse tree produced by KotlinParser#classParameters.
	VisitClassParameters(ctx *ClassParametersContext) interface{}

	// Visit a parse tree produced by KotlinParser#classParameter.
	VisitClassParameter(ctx *ClassParameterContext) interface{}

	// Visit a parse tree produced by KotlinParser#delegationSpecifiers.
	VisitDelegationSpecifiers(ctx *DelegationSpecifiersContext) interface{}

	// Visit a parse tree produced by KotlinParser#annotatedDelegationSpecifier.
	VisitAnnotatedDelegationSpecifier(ctx *AnnotatedDelegationSpecifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#delegationSpecifier.
	VisitDelegationSpecifier(ctx *DelegationSpecifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#constructorInvocation.
	VisitConstructorInvocation(ctx *ConstructorInvocationContext) interface{}

	// Visit a parse tree produced by KotlinParser#explicitDelegation.
	VisitExplicitDelegation(ctx *ExplicitDelegationContext) interface{}

	// Visit a parse tree produced by KotlinParser#classBody.
	VisitClassBody(ctx *ClassBodyContext) interface{}

	// Visit a parse tree produced by KotlinParser#classMemberDeclarations.
	VisitClassMemberDeclarations(ctx *ClassMemberDeclarationsContext) interface{}

	// Visit a parse tree produced by KotlinParser#classMemberDeclaration.
	VisitClassMemberDeclaration(ctx *ClassMemberDeclarationContext) interface{}

	// Visit a parse tree produced by KotlinParser#anonymousInitializer.
	VisitAnonymousInitializer(ctx *AnonymousInitializerContext) interface{}

	// Visit a parse tree produced by KotlinParser#secondaryConstructor.
	VisitSecondaryConstructor(ctx *SecondaryConstructorContext) interface{}

	// Visit a parse tree produced by KotlinParser#constructorDelegationCall.
	VisitConstructorDelegationCall(ctx *ConstructorDelegationCallContext) interface{}

	// Visit a parse tree produced by KotlinParser#enumClassBody.
	VisitEnumClassBody(ctx *EnumClassBodyContext) interface{}

	// Visit a parse tree produced by KotlinParser#enumEntries.
	VisitEnumEntries(ctx *EnumEntriesContext) interface{}

	// Visit a parse tree produced by KotlinParser#enumEntry.
	VisitEnumEntry(ctx *EnumEntryContext) interface{}

	// Visit a parse tree produced by KotlinParser#functionDeclaration.
	VisitFunctionDeclaration(ctx *FunctionDeclarationContext) interface{}

	// Visit a parse tree produced by KotlinParser#functionValueParameters.
	VisitFunctionValueParameters(ctx *FunctionValueParametersContext) interface{}

	// Visit a parse tree produced by KotlinParser#functionValueParameter.
	VisitFunctionValueParameter(ctx *FunctionValueParameterContext) interface{}

	// Visit a parse tree produced by KotlinParser#parameter.
	VisitParameter(ctx *ParameterContext) interface{}

	// Visit a parse tree produced by KotlinParser#setterParameter.
	VisitSetterParameter(ctx *SetterParameterContext) interface{}

	// Visit a parse tree produced by KotlinParser#functionBody.
	VisitFunctionBody(ctx *FunctionBodyContext) interface{}

	// Visit a parse tree produced by KotlinParser#objectDeclaration.
	VisitObjectDeclaration(ctx *ObjectDeclarationContext) interface{}

	// Visit a parse tree produced by KotlinParser#companionObject.
	VisitCompanionObject(ctx *CompanionObjectContext) interface{}

	// Visit a parse tree produced by KotlinParser#propertyDeclaration.
	VisitPropertyDeclaration(ctx *PropertyDeclarationContext) interface{}

	// Visit a parse tree produced by KotlinParser#multiVariableDeclaration.
	VisitMultiVariableDeclaration(ctx *MultiVariableDeclarationContext) interface{}

	// Visit a parse tree produced by KotlinParser#variableDeclaration.
	VisitVariableDeclaration(ctx *VariableDeclarationContext) interface{}

	// Visit a parse tree produced by KotlinParser#propertyDelegate.
	VisitPropertyDelegate(ctx *PropertyDelegateContext) interface{}

	// Visit a parse tree produced by KotlinParser#getter.
	VisitGetter(ctx *GetterContext) interface{}

	// Visit a parse tree produced by KotlinParser#setter.
	VisitSetter(ctx *SetterContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeAlias.
	VisitTypeAlias(ctx *TypeAliasContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeParameters.
	VisitTypeParameters(ctx *TypeParametersContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeParameter.
	VisitTypeParameter(ctx *TypeParameterContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeParameterModifiers.
	VisitTypeParameterModifiers(ctx *TypeParameterModifiersContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeParameterModifier.
	VisitTypeParameterModifier(ctx *TypeParameterModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#type_.
	VisitType_(ctx *Type_Context) interface{}

	// Visit a parse tree produced by KotlinParser#typeModifiers.
	VisitTypeModifiers(ctx *TypeModifiersContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeModifier.
	VisitTypeModifier(ctx *TypeModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#parenthesizedType.
	VisitParenthesizedType(ctx *ParenthesizedTypeContext) interface{}

	// Visit a parse tree produced by KotlinParser#nullableType.
	VisitNullableType(ctx *NullableTypeContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeReference.
	VisitTypeReference(ctx *TypeReferenceContext) interface{}

	// Visit a parse tree produced by KotlinParser#functionType.
	VisitFunctionType(ctx *FunctionTypeContext) interface{}

	// Visit a parse tree produced by KotlinParser#receiverType.
	VisitReceiverType(ctx *ReceiverTypeContext) interface{}

	// Visit a parse tree produced by KotlinParser#userType.
	VisitUserType(ctx *UserTypeContext) interface{}

	// Visit a parse tree produced by KotlinParser#parenthesizedUserType.
	VisitParenthesizedUserType(ctx *ParenthesizedUserTypeContext) interface{}

	// Visit a parse tree produced by KotlinParser#simpleUserType.
	VisitSimpleUserType(ctx *SimpleUserTypeContext) interface{}

	// Visit a parse tree produced by KotlinParser#functionTypeParameters.
	VisitFunctionTypeParameters(ctx *FunctionTypeParametersContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeConstraints.
	VisitTypeConstraints(ctx *TypeConstraintsContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeConstraint.
	VisitTypeConstraint(ctx *TypeConstraintContext) interface{}

	// Visit a parse tree produced by KotlinParser#block.
	VisitBlock(ctx *BlockContext) interface{}

	// Visit a parse tree produced by KotlinParser#statements.
	VisitStatements(ctx *StatementsContext) interface{}

	// Visit a parse tree produced by KotlinParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by KotlinParser#declaration.
	VisitDeclaration(ctx *DeclarationContext) interface{}

	// Visit a parse tree produced by KotlinParser#assignment.
	VisitAssignment(ctx *AssignmentContext) interface{}

	// Visit a parse tree produced by KotlinParser#expression.
	VisitExpression(ctx *ExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#disjunction.
	VisitDisjunction(ctx *DisjunctionContext) interface{}

	// Visit a parse tree produced by KotlinParser#conjunction.
	VisitConjunction(ctx *ConjunctionContext) interface{}

	// Visit a parse tree produced by KotlinParser#equality.
	VisitEquality(ctx *EqualityContext) interface{}

	// Visit a parse tree produced by KotlinParser#comparison.
	VisitComparison(ctx *ComparisonContext) interface{}

	// Visit a parse tree produced by KotlinParser#infixOperation.
	VisitInfixOperation(ctx *InfixOperationContext) interface{}

	// Visit a parse tree produced by KotlinParser#elvisExpression.
	VisitElvisExpression(ctx *ElvisExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#infixFunctionCall.
	VisitInfixFunctionCall(ctx *InfixFunctionCallContext) interface{}

	// Visit a parse tree produced by KotlinParser#rangeExpression.
	VisitRangeExpression(ctx *RangeExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#additiveExpression.
	VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#multiplicativeExpression.
	VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#asExpression.
	VisitAsExpression(ctx *AsExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#prefixUnaryExpression.
	VisitPrefixUnaryExpression(ctx *PrefixUnaryExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#unaryPrefix.
	VisitUnaryPrefix(ctx *UnaryPrefixContext) interface{}

	// Visit a parse tree produced by KotlinParser#postfixUnaryExpression.
	VisitPostfixUnaryExpression(ctx *PostfixUnaryExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#postfixUnarySuffix.
	VisitPostfixUnarySuffix(ctx *PostfixUnarySuffixContext) interface{}

	// Visit a parse tree produced by KotlinParser#directlyAssignableExpression.
	VisitDirectlyAssignableExpression(ctx *DirectlyAssignableExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#assignableExpression.
	VisitAssignableExpression(ctx *AssignableExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#assignableSuffix.
	VisitAssignableSuffix(ctx *AssignableSuffixContext) interface{}

	// Visit a parse tree produced by KotlinParser#indexingSuffix.
	VisitIndexingSuffix(ctx *IndexingSuffixContext) interface{}

	// Visit a parse tree produced by KotlinParser#navigationSuffix.
	VisitNavigationSuffix(ctx *NavigationSuffixContext) interface{}

	// Visit a parse tree produced by KotlinParser#callSuffix.
	VisitCallSuffix(ctx *CallSuffixContext) interface{}

	// Visit a parse tree produced by KotlinParser#annotatedLambda.
	VisitAnnotatedLambda(ctx *AnnotatedLambdaContext) interface{}

	// Visit a parse tree produced by KotlinParser#valueArguments.
	VisitValueArguments(ctx *ValueArgumentsContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeArguments.
	VisitTypeArguments(ctx *TypeArgumentsContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeProjection.
	VisitTypeProjection(ctx *TypeProjectionContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeProjectionModifiers.
	VisitTypeProjectionModifiers(ctx *TypeProjectionModifiersContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeProjectionModifier.
	VisitTypeProjectionModifier(ctx *TypeProjectionModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#valueArgument.
	VisitValueArgument(ctx *ValueArgumentContext) interface{}

	// Visit a parse tree produced by KotlinParser#primaryExpression.
	VisitPrimaryExpression(ctx *PrimaryExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#parenthesizedExpression.
	VisitParenthesizedExpression(ctx *ParenthesizedExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#collectionLiteral.
	VisitCollectionLiteral(ctx *CollectionLiteralContext) interface{}

	// Visit a parse tree produced by KotlinParser#literalConstant.
	VisitLiteralConstant(ctx *LiteralConstantContext) interface{}

	// Visit a parse tree produced by KotlinParser#stringLiteral.
	VisitStringLiteral(ctx *StringLiteralContext) interface{}

	// Visit a parse tree produced by KotlinParser#lineStringLiteral.
	VisitLineStringLiteral(ctx *LineStringLiteralContext) interface{}

	// Visit a parse tree produced by KotlinParser#multiLineStringLiteral.
	VisitMultiLineStringLiteral(ctx *MultiLineStringLiteralContext) interface{}

	// Visit a parse tree produced by KotlinParser#lineStringContent.
	VisitLineStringContent(ctx *LineStringContentContext) interface{}

	// Visit a parse tree produced by KotlinParser#lineStringExpression.
	VisitLineStringExpression(ctx *LineStringExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#multiLineStringContent.
	VisitMultiLineStringContent(ctx *MultiLineStringContentContext) interface{}

	// Visit a parse tree produced by KotlinParser#multiLineStringExpression.
	VisitMultiLineStringExpression(ctx *MultiLineStringExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#lambdaLiteral.
	VisitLambdaLiteral(ctx *LambdaLiteralContext) interface{}

	// Visit a parse tree produced by KotlinParser#lambdaParameters.
	VisitLambdaParameters(ctx *LambdaParametersContext) interface{}

	// Visit a parse tree produced by KotlinParser#lambdaParameter.
	VisitLambdaParameter(ctx *LambdaParameterContext) interface{}

	// Visit a parse tree produced by KotlinParser#anonymousFunction.
	VisitAnonymousFunction(ctx *AnonymousFunctionContext) interface{}

	// Visit a parse tree produced by KotlinParser#functionLiteral.
	VisitFunctionLiteral(ctx *FunctionLiteralContext) interface{}

	// Visit a parse tree produced by KotlinParser#objectLiteral.
	VisitObjectLiteral(ctx *ObjectLiteralContext) interface{}

	// Visit a parse tree produced by KotlinParser#thisExpression.
	VisitThisExpression(ctx *ThisExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#superExpression.
	VisitSuperExpression(ctx *SuperExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#controlStructureBody.
	VisitControlStructureBody(ctx *ControlStructureBodyContext) interface{}

	// Visit a parse tree produced by KotlinParser#ifExpression.
	VisitIfExpression(ctx *IfExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#whenExpression.
	VisitWhenExpression(ctx *WhenExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#whenEntry.
	VisitWhenEntry(ctx *WhenEntryContext) interface{}

	// Visit a parse tree produced by KotlinParser#whenCondition.
	VisitWhenCondition(ctx *WhenConditionContext) interface{}

	// Visit a parse tree produced by KotlinParser#rangeTest.
	VisitRangeTest(ctx *RangeTestContext) interface{}

	// Visit a parse tree produced by KotlinParser#typeTest.
	VisitTypeTest(ctx *TypeTestContext) interface{}

	// Visit a parse tree produced by KotlinParser#tryExpression.
	VisitTryExpression(ctx *TryExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#catchBlock.
	VisitCatchBlock(ctx *CatchBlockContext) interface{}

	// Visit a parse tree produced by KotlinParser#finallyBlock.
	VisitFinallyBlock(ctx *FinallyBlockContext) interface{}

	// Visit a parse tree produced by KotlinParser#loopStatement.
	VisitLoopStatement(ctx *LoopStatementContext) interface{}

	// Visit a parse tree produced by KotlinParser#forStatement.
	VisitForStatement(ctx *ForStatementContext) interface{}

	// Visit a parse tree produced by KotlinParser#whileStatement.
	VisitWhileStatement(ctx *WhileStatementContext) interface{}

	// Visit a parse tree produced by KotlinParser#doWhileStatement.
	VisitDoWhileStatement(ctx *DoWhileStatementContext) interface{}

	// Visit a parse tree produced by KotlinParser#jumpExpression.
	VisitJumpExpression(ctx *JumpExpressionContext) interface{}

	// Visit a parse tree produced by KotlinParser#callableReference.
	VisitCallableReference(ctx *CallableReferenceContext) interface{}

	// Visit a parse tree produced by KotlinParser#assignmentAndOperator.
	VisitAssignmentAndOperator(ctx *AssignmentAndOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#equalityOperator.
	VisitEqualityOperator(ctx *EqualityOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#comparisonOperator.
	VisitComparisonOperator(ctx *ComparisonOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#inOperator.
	VisitInOperator(ctx *InOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#isOperator.
	VisitIsOperator(ctx *IsOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#additiveOperator.
	VisitAdditiveOperator(ctx *AdditiveOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#multiplicativeOperator.
	VisitMultiplicativeOperator(ctx *MultiplicativeOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#asOperator.
	VisitAsOperator(ctx *AsOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#prefixUnaryOperator.
	VisitPrefixUnaryOperator(ctx *PrefixUnaryOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#postfixUnaryOperator.
	VisitPostfixUnaryOperator(ctx *PostfixUnaryOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#memberAccessOperator.
	VisitMemberAccessOperator(ctx *MemberAccessOperatorContext) interface{}

	// Visit a parse tree produced by KotlinParser#modifiers.
	VisitModifiers(ctx *ModifiersContext) interface{}

	// Visit a parse tree produced by KotlinParser#modifier.
	VisitModifier(ctx *ModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#classModifier.
	VisitClassModifier(ctx *ClassModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#memberModifier.
	VisitMemberModifier(ctx *MemberModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#visibilityModifier.
	VisitVisibilityModifier(ctx *VisibilityModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#varianceModifier.
	VisitVarianceModifier(ctx *VarianceModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#functionModifier.
	VisitFunctionModifier(ctx *FunctionModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#propertyModifier.
	VisitPropertyModifier(ctx *PropertyModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#inheritanceModifier.
	VisitInheritanceModifier(ctx *InheritanceModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#parameterModifier.
	VisitParameterModifier(ctx *ParameterModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#reificationModifier.
	VisitReificationModifier(ctx *ReificationModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#platformModifier.
	VisitPlatformModifier(ctx *PlatformModifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#label.
	VisitLabel(ctx *LabelContext) interface{}

	// Visit a parse tree produced by KotlinParser#annotation.
	VisitAnnotation(ctx *AnnotationContext) interface{}

	// Visit a parse tree produced by KotlinParser#singleAnnotation.
	VisitSingleAnnotation(ctx *SingleAnnotationContext) interface{}

	// Visit a parse tree produced by KotlinParser#multiAnnotation.
	VisitMultiAnnotation(ctx *MultiAnnotationContext) interface{}

	// Visit a parse tree produced by KotlinParser#annotationUseSiteTarget.
	VisitAnnotationUseSiteTarget(ctx *AnnotationUseSiteTargetContext) interface{}

	// Visit a parse tree produced by KotlinParser#unescapedAnnotation.
	VisitUnescapedAnnotation(ctx *UnescapedAnnotationContext) interface{}

	// Visit a parse tree produced by KotlinParser#simpleIdentifier.
	VisitSimpleIdentifier(ctx *SimpleIdentifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#identifier.
	VisitIdentifier(ctx *IdentifierContext) interface{}

	// Visit a parse tree produced by KotlinParser#shebangLine.
	VisitShebangLine(ctx *ShebangLineContext) interface{}

	// Visit a parse tree produced by KotlinParser#quest.
	VisitQuest(ctx *QuestContext) interface{}

	// Visit a parse tree produced by KotlinParser#elvis.
	VisitElvis(ctx *ElvisContext) interface{}

	// Visit a parse tree produced by KotlinParser#safeNav.
	VisitSafeNav(ctx *SafeNavContext) interface{}

	// Visit a parse tree produced by KotlinParser#excl.
	VisitExcl(ctx *ExclContext) interface{}

	// Visit a parse tree produced by KotlinParser#semi.
	VisitSemi(ctx *SemiContext) interface{}

	// Visit a parse tree produced by KotlinParser#semis.
	VisitSemis(ctx *SemisContext) interface{}
}
