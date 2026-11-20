// Code generated from ./CPPParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package cppparser // CPPParser
import "github.com/yaklang/antlr/v4"

// A complete Visitor for a parse tree produced by CPPParser.
type CPPParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by CPPParser#translationUnit.
	VisitTranslationUnit(ctx *TranslationUnitContext) interface{}

	// Visit a parse tree produced by CPPParser#primaryExpression.
	VisitPrimaryExpression(ctx *PrimaryExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#foldExpression.
	VisitFoldExpression(ctx *FoldExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#foldOperator.
	VisitFoldOperator(ctx *FoldOperatorContext) interface{}

	// Visit a parse tree produced by CPPParser#idExpression.
	VisitIdExpression(ctx *IdExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#unqualifiedId.
	VisitUnqualifiedId(ctx *UnqualifiedIdContext) interface{}

	// Visit a parse tree produced by CPPParser#qualifiedId.
	VisitQualifiedId(ctx *QualifiedIdContext) interface{}

	// Visit a parse tree produced by CPPParser#nestedNameSpecifier.
	VisitNestedNameSpecifier(ctx *NestedNameSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#lambdaExpression.
	VisitLambdaExpression(ctx *LambdaExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#lambdaIntroducer.
	VisitLambdaIntroducer(ctx *LambdaIntroducerContext) interface{}

	// Visit a parse tree produced by CPPParser#lambdaCapture.
	VisitLambdaCapture(ctx *LambdaCaptureContext) interface{}

	// Visit a parse tree produced by CPPParser#captureDefault.
	VisitCaptureDefault(ctx *CaptureDefaultContext) interface{}

	// Visit a parse tree produced by CPPParser#captureList.
	VisitCaptureList(ctx *CaptureListContext) interface{}

	// Visit a parse tree produced by CPPParser#capture.
	VisitCapture(ctx *CaptureContext) interface{}

	// Visit a parse tree produced by CPPParser#simpleCapture.
	VisitSimpleCapture(ctx *SimpleCaptureContext) interface{}

	// Visit a parse tree produced by CPPParser#initCapture.
	VisitInitCapture(ctx *InitCaptureContext) interface{}

	// Visit a parse tree produced by CPPParser#lambdaDeclarator.
	VisitLambdaDeclarator(ctx *LambdaDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#postfixExpression.
	VisitPostfixExpression(ctx *PostfixExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#typeIdOfTheTypeId.
	VisitTypeIdOfTheTypeId(ctx *TypeIdOfTheTypeIdContext) interface{}

	// Visit a parse tree produced by CPPParser#expressionList.
	VisitExpressionList(ctx *ExpressionListContext) interface{}

	// Visit a parse tree produced by CPPParser#pseudoDestructorName.
	VisitPseudoDestructorName(ctx *PseudoDestructorNameContext) interface{}

	// Visit a parse tree produced by CPPParser#unaryExpression.
	VisitUnaryExpression(ctx *UnaryExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#unaryOperator.
	VisitUnaryOperator(ctx *UnaryOperatorContext) interface{}

	// Visit a parse tree produced by CPPParser#newExpression_.
	VisitNewExpression_(ctx *NewExpression_Context) interface{}

	// Visit a parse tree produced by CPPParser#newPlacement.
	VisitNewPlacement(ctx *NewPlacementContext) interface{}

	// Visit a parse tree produced by CPPParser#newTypeId.
	VisitNewTypeId(ctx *NewTypeIdContext) interface{}

	// Visit a parse tree produced by CPPParser#newDeclarator_.
	VisitNewDeclarator_(ctx *NewDeclarator_Context) interface{}

	// Visit a parse tree produced by CPPParser#noPointerNewDeclarator.
	VisitNoPointerNewDeclarator(ctx *NoPointerNewDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#newInitializer_.
	VisitNewInitializer_(ctx *NewInitializer_Context) interface{}

	// Visit a parse tree produced by CPPParser#deleteExpression.
	VisitDeleteExpression(ctx *DeleteExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#noExceptExpression.
	VisitNoExceptExpression(ctx *NoExceptExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#castExpression.
	VisitCastExpression(ctx *CastExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#pointerMemberExpression.
	VisitPointerMemberExpression(ctx *PointerMemberExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#multiplicativeExpression.
	VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#additiveExpression.
	VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#shiftExpression.
	VisitShiftExpression(ctx *ShiftExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#shiftOperator.
	VisitShiftOperator(ctx *ShiftOperatorContext) interface{}

	// Visit a parse tree produced by CPPParser#relationalExpression.
	VisitRelationalExpression(ctx *RelationalExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#equalityExpression.
	VisitEqualityExpression(ctx *EqualityExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#andExpression.
	VisitAndExpression(ctx *AndExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#exclusiveOrExpression.
	VisitExclusiveOrExpression(ctx *ExclusiveOrExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#inclusiveOrExpression.
	VisitInclusiveOrExpression(ctx *InclusiveOrExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#logicalAndExpression.
	VisitLogicalAndExpression(ctx *LogicalAndExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#logicalOrExpression.
	VisitLogicalOrExpression(ctx *LogicalOrExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#conditionalExpression.
	VisitConditionalExpression(ctx *ConditionalExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#assignmentExpression.
	VisitAssignmentExpression(ctx *AssignmentExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#assignmentOperator.
	VisitAssignmentOperator(ctx *AssignmentOperatorContext) interface{}

	// Visit a parse tree produced by CPPParser#expression.
	VisitExpression(ctx *ExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#constantExpression.
	VisitConstantExpression(ctx *ConstantExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by CPPParser#labeledStatement.
	VisitLabeledStatement(ctx *LabeledStatementContext) interface{}

	// Visit a parse tree produced by CPPParser#expressionStatement.
	VisitExpressionStatement(ctx *ExpressionStatementContext) interface{}

	// Visit a parse tree produced by CPPParser#compoundStatement.
	VisitCompoundStatement(ctx *CompoundStatementContext) interface{}

	// Visit a parse tree produced by CPPParser#statementSeq.
	VisitStatementSeq(ctx *StatementSeqContext) interface{}

	// Visit a parse tree produced by CPPParser#selectionStatement.
	VisitSelectionStatement(ctx *SelectionStatementContext) interface{}

	// Visit a parse tree produced by CPPParser#condition.
	VisitCondition(ctx *ConditionContext) interface{}

	// Visit a parse tree produced by CPPParser#iterationStatement.
	VisitIterationStatement(ctx *IterationStatementContext) interface{}

	// Visit a parse tree produced by CPPParser#forInitStatement.
	VisitForInitStatement(ctx *ForInitStatementContext) interface{}

	// Visit a parse tree produced by CPPParser#forRangeDeclaration.
	VisitForRangeDeclaration(ctx *ForRangeDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#forRangeInitializer.
	VisitForRangeInitializer(ctx *ForRangeInitializerContext) interface{}

	// Visit a parse tree produced by CPPParser#jumpStatement.
	VisitJumpStatement(ctx *JumpStatementContext) interface{}

	// Visit a parse tree produced by CPPParser#declarationStatement.
	VisitDeclarationStatement(ctx *DeclarationStatementContext) interface{}

	// Visit a parse tree produced by CPPParser#declarationSeq.
	VisitDeclarationSeq(ctx *DeclarationSeqContext) interface{}

	// Visit a parse tree produced by CPPParser#declaration.
	VisitDeclaration(ctx *DeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#blockDeclaration.
	VisitBlockDeclaration(ctx *BlockDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#aliasDeclaration.
	VisitAliasDeclaration(ctx *AliasDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#simpleDeclaration.
	VisitSimpleDeclaration(ctx *SimpleDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#identifierList.
	VisitIdentifierList(ctx *IdentifierListContext) interface{}

	// Visit a parse tree produced by CPPParser#staticAssertDeclaration.
	VisitStaticAssertDeclaration(ctx *StaticAssertDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#emptyDeclaration_.
	VisitEmptyDeclaration_(ctx *EmptyDeclaration_Context) interface{}

	// Visit a parse tree produced by CPPParser#attributeDeclaration.
	VisitAttributeDeclaration(ctx *AttributeDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#declSpecifier.
	VisitDeclSpecifier(ctx *DeclSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#declSpecifierSeq.
	VisitDeclSpecifierSeq(ctx *DeclSpecifierSeqContext) interface{}

	// Visit a parse tree produced by CPPParser#storageClassSpecifier.
	VisitStorageClassSpecifier(ctx *StorageClassSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#functionSpecifier.
	VisitFunctionSpecifier(ctx *FunctionSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#typedefName.
	VisitTypedefName(ctx *TypedefNameContext) interface{}

	// Visit a parse tree produced by CPPParser#typeSpecifier.
	VisitTypeSpecifier(ctx *TypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#trailingTypeSpecifier.
	VisitTrailingTypeSpecifier(ctx *TrailingTypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#typeSpecifierSeq.
	VisitTypeSpecifierSeq(ctx *TypeSpecifierSeqContext) interface{}

	// Visit a parse tree produced by CPPParser#trailingTypeSpecifierSeq.
	VisitTrailingTypeSpecifierSeq(ctx *TrailingTypeSpecifierSeqContext) interface{}

	// Visit a parse tree produced by CPPParser#simpleTypeLengthModifier.
	VisitSimpleTypeLengthModifier(ctx *SimpleTypeLengthModifierContext) interface{}

	// Visit a parse tree produced by CPPParser#simpleTypeSignednessModifier.
	VisitSimpleTypeSignednessModifier(ctx *SimpleTypeSignednessModifierContext) interface{}

	// Visit a parse tree produced by CPPParser#simpleTypeSpecifier.
	VisitSimpleTypeSpecifier(ctx *SimpleTypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#theTypeName.
	VisitTheTypeName(ctx *TheTypeNameContext) interface{}

	// Visit a parse tree produced by CPPParser#decltypeSpecifier.
	VisitDecltypeSpecifier(ctx *DecltypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#elaboratedTypeSpecifier.
	VisitElaboratedTypeSpecifier(ctx *ElaboratedTypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#enumName.
	VisitEnumName(ctx *EnumNameContext) interface{}

	// Visit a parse tree produced by CPPParser#enumSpecifier.
	VisitEnumSpecifier(ctx *EnumSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#enumHead.
	VisitEnumHead(ctx *EnumHeadContext) interface{}

	// Visit a parse tree produced by CPPParser#opaqueEnumDeclaration.
	VisitOpaqueEnumDeclaration(ctx *OpaqueEnumDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#enumKey.
	VisitEnumKey(ctx *EnumKeyContext) interface{}

	// Visit a parse tree produced by CPPParser#enumBase.
	VisitEnumBase(ctx *EnumBaseContext) interface{}

	// Visit a parse tree produced by CPPParser#enumeratorList.
	VisitEnumeratorList(ctx *EnumeratorListContext) interface{}

	// Visit a parse tree produced by CPPParser#enumeratorDefinition.
	VisitEnumeratorDefinition(ctx *EnumeratorDefinitionContext) interface{}

	// Visit a parse tree produced by CPPParser#enumerator.
	VisitEnumerator(ctx *EnumeratorContext) interface{}

	// Visit a parse tree produced by CPPParser#namespaceName.
	VisitNamespaceName(ctx *NamespaceNameContext) interface{}

	// Visit a parse tree produced by CPPParser#originalNamespaceName.
	VisitOriginalNamespaceName(ctx *OriginalNamespaceNameContext) interface{}

	// Visit a parse tree produced by CPPParser#namespaceDefinition.
	VisitNamespaceDefinition(ctx *NamespaceDefinitionContext) interface{}

	// Visit a parse tree produced by CPPParser#namespaceAlias.
	VisitNamespaceAlias(ctx *NamespaceAliasContext) interface{}

	// Visit a parse tree produced by CPPParser#namespaceAliasDefinition.
	VisitNamespaceAliasDefinition(ctx *NamespaceAliasDefinitionContext) interface{}

	// Visit a parse tree produced by CPPParser#qualifiedNamespaceSpecifier.
	VisitQualifiedNamespaceSpecifier(ctx *QualifiedNamespaceSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#usingDeclaration.
	VisitUsingDeclaration(ctx *UsingDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#usingDirective.
	VisitUsingDirective(ctx *UsingDirectiveContext) interface{}

	// Visit a parse tree produced by CPPParser#asmDefinition.
	VisitAsmDefinition(ctx *AsmDefinitionContext) interface{}

	// Visit a parse tree produced by CPPParser#linkageSpecification.
	VisitLinkageSpecification(ctx *LinkageSpecificationContext) interface{}

	// Visit a parse tree produced by CPPParser#attributeSpecifierSeq.
	VisitAttributeSpecifierSeq(ctx *AttributeSpecifierSeqContext) interface{}

	// Visit a parse tree produced by CPPParser#attributeSpecifier.
	VisitAttributeSpecifier(ctx *AttributeSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#alignmentSpecifier.
	VisitAlignmentSpecifier(ctx *AlignmentSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#attributeList.
	VisitAttributeList(ctx *AttributeListContext) interface{}

	// Visit a parse tree produced by CPPParser#attribute.
	VisitAttribute(ctx *AttributeContext) interface{}

	// Visit a parse tree produced by CPPParser#attributeNamespace.
	VisitAttributeNamespace(ctx *AttributeNamespaceContext) interface{}

	// Visit a parse tree produced by CPPParser#attributeArgumentClause.
	VisitAttributeArgumentClause(ctx *AttributeArgumentClauseContext) interface{}

	// Visit a parse tree produced by CPPParser#balancedTokenSeq.
	VisitBalancedTokenSeq(ctx *BalancedTokenSeqContext) interface{}

	// Visit a parse tree produced by CPPParser#balancedToken.
	VisitBalancedToken(ctx *BalancedTokenContext) interface{}

	// Visit a parse tree produced by CPPParser#initDeclaratorList.
	VisitInitDeclaratorList(ctx *InitDeclaratorListContext) interface{}

	// Visit a parse tree produced by CPPParser#initDeclarator.
	VisitInitDeclarator(ctx *InitDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#declarator.
	VisitDeclarator(ctx *DeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#pointerDeclarator.
	VisitPointerDeclarator(ctx *PointerDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#noPointerDeclarator.
	VisitNoPointerDeclarator(ctx *NoPointerDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#parametersAndQualifiers.
	VisitParametersAndQualifiers(ctx *ParametersAndQualifiersContext) interface{}

	// Visit a parse tree produced by CPPParser#trailingReturnType.
	VisitTrailingReturnType(ctx *TrailingReturnTypeContext) interface{}

	// Visit a parse tree produced by CPPParser#pointerOperator.
	VisitPointerOperator(ctx *PointerOperatorContext) interface{}

	// Visit a parse tree produced by CPPParser#cvQualifierSeq.
	VisitCvQualifierSeq(ctx *CvQualifierSeqContext) interface{}

	// Visit a parse tree produced by CPPParser#cvQualifier.
	VisitCvQualifier(ctx *CvQualifierContext) interface{}

	// Visit a parse tree produced by CPPParser#refQualifier.
	VisitRefQualifier(ctx *RefQualifierContext) interface{}

	// Visit a parse tree produced by CPPParser#declaratorId.
	VisitDeclaratorId(ctx *DeclaratorIdContext) interface{}

	// Visit a parse tree produced by CPPParser#theTypeId.
	VisitTheTypeId(ctx *TheTypeIdContext) interface{}

	// Visit a parse tree produced by CPPParser#abstractDeclarator.
	VisitAbstractDeclarator(ctx *AbstractDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#pointerAbstractDeclarator.
	VisitPointerAbstractDeclarator(ctx *PointerAbstractDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#noPointerAbstractDeclarator.
	VisitNoPointerAbstractDeclarator(ctx *NoPointerAbstractDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#abstractPackDeclarator.
	VisitAbstractPackDeclarator(ctx *AbstractPackDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#noPointerAbstractPackDeclarator.
	VisitNoPointerAbstractPackDeclarator(ctx *NoPointerAbstractPackDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#parameterDeclarationClause.
	VisitParameterDeclarationClause(ctx *ParameterDeclarationClauseContext) interface{}

	// Visit a parse tree produced by CPPParser#parameterDeclarationList.
	VisitParameterDeclarationList(ctx *ParameterDeclarationListContext) interface{}

	// Visit a parse tree produced by CPPParser#parameterDeclaration.
	VisitParameterDeclaration(ctx *ParameterDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#functionDefinition.
	VisitFunctionDefinition(ctx *FunctionDefinitionContext) interface{}

	// Visit a parse tree produced by CPPParser#functionBody.
	VisitFunctionBody(ctx *FunctionBodyContext) interface{}

	// Visit a parse tree produced by CPPParser#initializer.
	VisitInitializer(ctx *InitializerContext) interface{}

	// Visit a parse tree produced by CPPParser#braceOrEqualInitializer.
	VisitBraceOrEqualInitializer(ctx *BraceOrEqualInitializerContext) interface{}

	// Visit a parse tree produced by CPPParser#initializerClause.
	VisitInitializerClause(ctx *InitializerClauseContext) interface{}

	// Visit a parse tree produced by CPPParser#initializerList.
	VisitInitializerList(ctx *InitializerListContext) interface{}

	// Visit a parse tree produced by CPPParser#bracedInitList.
	VisitBracedInitList(ctx *BracedInitListContext) interface{}

	// Visit a parse tree produced by CPPParser#className.
	VisitClassName(ctx *ClassNameContext) interface{}

	// Visit a parse tree produced by CPPParser#classSpecifier.
	VisitClassSpecifier(ctx *ClassSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#classHead.
	VisitClassHead(ctx *ClassHeadContext) interface{}

	// Visit a parse tree produced by CPPParser#classHeadName.
	VisitClassHeadName(ctx *ClassHeadNameContext) interface{}

	// Visit a parse tree produced by CPPParser#classVirtSpecifier.
	VisitClassVirtSpecifier(ctx *ClassVirtSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#classKey.
	VisitClassKey(ctx *ClassKeyContext) interface{}

	// Visit a parse tree produced by CPPParser#memberSpecification.
	VisitMemberSpecification(ctx *MemberSpecificationContext) interface{}

	// Visit a parse tree produced by CPPParser#memberDeclaration.
	VisitMemberDeclaration(ctx *MemberDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#memberDeclaratorList.
	VisitMemberDeclaratorList(ctx *MemberDeclaratorListContext) interface{}

	// Visit a parse tree produced by CPPParser#memberDeclarator.
	VisitMemberDeclarator(ctx *MemberDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#virtualSpecifierSeq.
	VisitVirtualSpecifierSeq(ctx *VirtualSpecifierSeqContext) interface{}

	// Visit a parse tree produced by CPPParser#virtualSpecifier.
	VisitVirtualSpecifier(ctx *VirtualSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#pureSpecifier.
	VisitPureSpecifier(ctx *PureSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#baseClause.
	VisitBaseClause(ctx *BaseClauseContext) interface{}

	// Visit a parse tree produced by CPPParser#baseSpecifierList.
	VisitBaseSpecifierList(ctx *BaseSpecifierListContext) interface{}

	// Visit a parse tree produced by CPPParser#baseSpecifier.
	VisitBaseSpecifier(ctx *BaseSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#classOrDeclType.
	VisitClassOrDeclType(ctx *ClassOrDeclTypeContext) interface{}

	// Visit a parse tree produced by CPPParser#baseTypeSpecifier.
	VisitBaseTypeSpecifier(ctx *BaseTypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#accessSpecifier.
	VisitAccessSpecifier(ctx *AccessSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#conversionFunctionId.
	VisitConversionFunctionId(ctx *ConversionFunctionIdContext) interface{}

	// Visit a parse tree produced by CPPParser#conversionTypeId.
	VisitConversionTypeId(ctx *ConversionTypeIdContext) interface{}

	// Visit a parse tree produced by CPPParser#conversionDeclarator.
	VisitConversionDeclarator(ctx *ConversionDeclaratorContext) interface{}

	// Visit a parse tree produced by CPPParser#constructorInitializer.
	VisitConstructorInitializer(ctx *ConstructorInitializerContext) interface{}

	// Visit a parse tree produced by CPPParser#memInitializerList.
	VisitMemInitializerList(ctx *MemInitializerListContext) interface{}

	// Visit a parse tree produced by CPPParser#memInitializer.
	VisitMemInitializer(ctx *MemInitializerContext) interface{}

	// Visit a parse tree produced by CPPParser#memInitializerId.
	VisitMemInitializerId(ctx *MemInitializerIdContext) interface{}

	// Visit a parse tree produced by CPPParser#operatorFunctionId.
	VisitOperatorFunctionId(ctx *OperatorFunctionIdContext) interface{}

	// Visit a parse tree produced by CPPParser#literalOperatorId.
	VisitLiteralOperatorId(ctx *LiteralOperatorIdContext) interface{}

	// Visit a parse tree produced by CPPParser#templateDeclaration.
	VisitTemplateDeclaration(ctx *TemplateDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#templateParameterList.
	VisitTemplateParameterList(ctx *TemplateParameterListContext) interface{}

	// Visit a parse tree produced by CPPParser#templateParameter.
	VisitTemplateParameter(ctx *TemplateParameterContext) interface{}

	// Visit a parse tree produced by CPPParser#typeParameter.
	VisitTypeParameter(ctx *TypeParameterContext) interface{}

	// Visit a parse tree produced by CPPParser#simpleTemplateId.
	VisitSimpleTemplateId(ctx *SimpleTemplateIdContext) interface{}

	// Visit a parse tree produced by CPPParser#templateId.
	VisitTemplateId(ctx *TemplateIdContext) interface{}

	// Visit a parse tree produced by CPPParser#templateName.
	VisitTemplateName(ctx *TemplateNameContext) interface{}

	// Visit a parse tree produced by CPPParser#templateArgumentList.
	VisitTemplateArgumentList(ctx *TemplateArgumentListContext) interface{}

	// Visit a parse tree produced by CPPParser#templateArgument.
	VisitTemplateArgument(ctx *TemplateArgumentContext) interface{}

	// Visit a parse tree produced by CPPParser#typeNameSpecifier.
	VisitTypeNameSpecifier(ctx *TypeNameSpecifierContext) interface{}

	// Visit a parse tree produced by CPPParser#explicitInstantiation.
	VisitExplicitInstantiation(ctx *ExplicitInstantiationContext) interface{}

	// Visit a parse tree produced by CPPParser#explicitSpecialization.
	VisitExplicitSpecialization(ctx *ExplicitSpecializationContext) interface{}

	// Visit a parse tree produced by CPPParser#tryBlock.
	VisitTryBlock(ctx *TryBlockContext) interface{}

	// Visit a parse tree produced by CPPParser#functionTryBlock.
	VisitFunctionTryBlock(ctx *FunctionTryBlockContext) interface{}

	// Visit a parse tree produced by CPPParser#handlerSeq.
	VisitHandlerSeq(ctx *HandlerSeqContext) interface{}

	// Visit a parse tree produced by CPPParser#handler.
	VisitHandler(ctx *HandlerContext) interface{}

	// Visit a parse tree produced by CPPParser#exceptionDeclaration.
	VisitExceptionDeclaration(ctx *ExceptionDeclarationContext) interface{}

	// Visit a parse tree produced by CPPParser#throwExpression.
	VisitThrowExpression(ctx *ThrowExpressionContext) interface{}

	// Visit a parse tree produced by CPPParser#exceptionSpecification.
	VisitExceptionSpecification(ctx *ExceptionSpecificationContext) interface{}

	// Visit a parse tree produced by CPPParser#dynamicExceptionSpecification.
	VisitDynamicExceptionSpecification(ctx *DynamicExceptionSpecificationContext) interface{}

	// Visit a parse tree produced by CPPParser#typeIdList.
	VisitTypeIdList(ctx *TypeIdListContext) interface{}

	// Visit a parse tree produced by CPPParser#noExceptSpecification.
	VisitNoExceptSpecification(ctx *NoExceptSpecificationContext) interface{}

	// Visit a parse tree produced by CPPParser#theOperator.
	VisitTheOperator(ctx *TheOperatorContext) interface{}

	// Visit a parse tree produced by CPPParser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}
}
