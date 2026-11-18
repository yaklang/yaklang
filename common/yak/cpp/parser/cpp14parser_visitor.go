// Code generated from ./CPP14Parser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package cppparser // CPP14Parser
import "github.com/yaklang/antlr/v4"

// A complete Visitor for a parse tree produced by CPP14Parser.
type CPP14ParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by CPP14Parser#translationUnit.
	VisitTranslationUnit(ctx *TranslationUnitContext) interface{}

	// Visit a parse tree produced by CPP14Parser#primaryExpression.
	VisitPrimaryExpression(ctx *PrimaryExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#foldExpression.
	VisitFoldExpression(ctx *FoldExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#foldOperator.
	VisitFoldOperator(ctx *FoldOperatorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#idExpression.
	VisitIdExpression(ctx *IdExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#unqualifiedId.
	VisitUnqualifiedId(ctx *UnqualifiedIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#qualifiedId.
	VisitQualifiedId(ctx *QualifiedIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#nestedNameSpecifier.
	VisitNestedNameSpecifier(ctx *NestedNameSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#lambdaExpression.
	VisitLambdaExpression(ctx *LambdaExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#lambdaIntroducer.
	VisitLambdaIntroducer(ctx *LambdaIntroducerContext) interface{}

	// Visit a parse tree produced by CPP14Parser#lambdaCapture.
	VisitLambdaCapture(ctx *LambdaCaptureContext) interface{}

	// Visit a parse tree produced by CPP14Parser#captureDefault.
	VisitCaptureDefault(ctx *CaptureDefaultContext) interface{}

	// Visit a parse tree produced by CPP14Parser#captureList.
	VisitCaptureList(ctx *CaptureListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#capture.
	VisitCapture(ctx *CaptureContext) interface{}

	// Visit a parse tree produced by CPP14Parser#simpleCapture.
	VisitSimpleCapture(ctx *SimpleCaptureContext) interface{}

	// Visit a parse tree produced by CPP14Parser#initCapture.
	VisitInitCapture(ctx *InitCaptureContext) interface{}

	// Visit a parse tree produced by CPP14Parser#lambdaDeclarator.
	VisitLambdaDeclarator(ctx *LambdaDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#postfixExpression.
	VisitPostfixExpression(ctx *PostfixExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#typeIdOfTheTypeId.
	VisitTypeIdOfTheTypeId(ctx *TypeIdOfTheTypeIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#expressionList.
	VisitExpressionList(ctx *ExpressionListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#pseudoDestructorName.
	VisitPseudoDestructorName(ctx *PseudoDestructorNameContext) interface{}

	// Visit a parse tree produced by CPP14Parser#unaryExpression.
	VisitUnaryExpression(ctx *UnaryExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#unaryOperator.
	VisitUnaryOperator(ctx *UnaryOperatorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#newExpression_.
	VisitNewExpression_(ctx *NewExpression_Context) interface{}

	// Visit a parse tree produced by CPP14Parser#newPlacement.
	VisitNewPlacement(ctx *NewPlacementContext) interface{}

	// Visit a parse tree produced by CPP14Parser#newTypeId.
	VisitNewTypeId(ctx *NewTypeIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#newDeclarator_.
	VisitNewDeclarator_(ctx *NewDeclarator_Context) interface{}

	// Visit a parse tree produced by CPP14Parser#noPointerNewDeclarator.
	VisitNoPointerNewDeclarator(ctx *NoPointerNewDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#newInitializer_.
	VisitNewInitializer_(ctx *NewInitializer_Context) interface{}

	// Visit a parse tree produced by CPP14Parser#deleteExpression.
	VisitDeleteExpression(ctx *DeleteExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#noExceptExpression.
	VisitNoExceptExpression(ctx *NoExceptExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#castExpression.
	VisitCastExpression(ctx *CastExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#pointerMemberExpression.
	VisitPointerMemberExpression(ctx *PointerMemberExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#multiplicativeExpression.
	VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#additiveExpression.
	VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#shiftExpression.
	VisitShiftExpression(ctx *ShiftExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#shiftOperator.
	VisitShiftOperator(ctx *ShiftOperatorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#relationalExpression.
	VisitRelationalExpression(ctx *RelationalExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#equalityExpression.
	VisitEqualityExpression(ctx *EqualityExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#andExpression.
	VisitAndExpression(ctx *AndExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#exclusiveOrExpression.
	VisitExclusiveOrExpression(ctx *ExclusiveOrExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#inclusiveOrExpression.
	VisitInclusiveOrExpression(ctx *InclusiveOrExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#logicalAndExpression.
	VisitLogicalAndExpression(ctx *LogicalAndExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#logicalOrExpression.
	VisitLogicalOrExpression(ctx *LogicalOrExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#conditionalExpression.
	VisitConditionalExpression(ctx *ConditionalExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#assignmentExpression.
	VisitAssignmentExpression(ctx *AssignmentExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#assignmentOperator.
	VisitAssignmentOperator(ctx *AssignmentOperatorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#expression.
	VisitExpression(ctx *ExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#constantExpression.
	VisitConstantExpression(ctx *ConstantExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by CPP14Parser#labeledStatement.
	VisitLabeledStatement(ctx *LabeledStatementContext) interface{}

	// Visit a parse tree produced by CPP14Parser#expressionStatement.
	VisitExpressionStatement(ctx *ExpressionStatementContext) interface{}

	// Visit a parse tree produced by CPP14Parser#compoundStatement.
	VisitCompoundStatement(ctx *CompoundStatementContext) interface{}

	// Visit a parse tree produced by CPP14Parser#statementSeq.
	VisitStatementSeq(ctx *StatementSeqContext) interface{}

	// Visit a parse tree produced by CPP14Parser#selectionStatement.
	VisitSelectionStatement(ctx *SelectionStatementContext) interface{}

	// Visit a parse tree produced by CPP14Parser#condition.
	VisitCondition(ctx *ConditionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#iterationStatement.
	VisitIterationStatement(ctx *IterationStatementContext) interface{}

	// Visit a parse tree produced by CPP14Parser#forInitStatement.
	VisitForInitStatement(ctx *ForInitStatementContext) interface{}

	// Visit a parse tree produced by CPP14Parser#forRangeDeclaration.
	VisitForRangeDeclaration(ctx *ForRangeDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#forRangeInitializer.
	VisitForRangeInitializer(ctx *ForRangeInitializerContext) interface{}

	// Visit a parse tree produced by CPP14Parser#jumpStatement.
	VisitJumpStatement(ctx *JumpStatementContext) interface{}

	// Visit a parse tree produced by CPP14Parser#declarationStatement.
	VisitDeclarationStatement(ctx *DeclarationStatementContext) interface{}

	// Visit a parse tree produced by CPP14Parser#declarationSeq.
	VisitDeclarationSeq(ctx *DeclarationSeqContext) interface{}

	// Visit a parse tree produced by CPP14Parser#declaration.
	VisitDeclaration(ctx *DeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#blockDeclaration.
	VisitBlockDeclaration(ctx *BlockDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#aliasDeclaration.
	VisitAliasDeclaration(ctx *AliasDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#simpleDeclaration.
	VisitSimpleDeclaration(ctx *SimpleDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#identifierList.
	VisitIdentifierList(ctx *IdentifierListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#staticAssertDeclaration.
	VisitStaticAssertDeclaration(ctx *StaticAssertDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#emptyDeclaration_.
	VisitEmptyDeclaration_(ctx *EmptyDeclaration_Context) interface{}

	// Visit a parse tree produced by CPP14Parser#attributeDeclaration.
	VisitAttributeDeclaration(ctx *AttributeDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#declSpecifier.
	VisitDeclSpecifier(ctx *DeclSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#declSpecifierSeq.
	VisitDeclSpecifierSeq(ctx *DeclSpecifierSeqContext) interface{}

	// Visit a parse tree produced by CPP14Parser#storageClassSpecifier.
	VisitStorageClassSpecifier(ctx *StorageClassSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#functionSpecifier.
	VisitFunctionSpecifier(ctx *FunctionSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#typedefName.
	VisitTypedefName(ctx *TypedefNameContext) interface{}

	// Visit a parse tree produced by CPP14Parser#typeSpecifier.
	VisitTypeSpecifier(ctx *TypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#trailingTypeSpecifier.
	VisitTrailingTypeSpecifier(ctx *TrailingTypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#typeSpecifierSeq.
	VisitTypeSpecifierSeq(ctx *TypeSpecifierSeqContext) interface{}

	// Visit a parse tree produced by CPP14Parser#trailingTypeSpecifierSeq.
	VisitTrailingTypeSpecifierSeq(ctx *TrailingTypeSpecifierSeqContext) interface{}

	// Visit a parse tree produced by CPP14Parser#simpleTypeLengthModifier.
	VisitSimpleTypeLengthModifier(ctx *SimpleTypeLengthModifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#simpleTypeSignednessModifier.
	VisitSimpleTypeSignednessModifier(ctx *SimpleTypeSignednessModifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#simpleTypeSpecifier.
	VisitSimpleTypeSpecifier(ctx *SimpleTypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#theTypeName.
	VisitTheTypeName(ctx *TheTypeNameContext) interface{}

	// Visit a parse tree produced by CPP14Parser#decltypeSpecifier.
	VisitDecltypeSpecifier(ctx *DecltypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#elaboratedTypeSpecifier.
	VisitElaboratedTypeSpecifier(ctx *ElaboratedTypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#enumName.
	VisitEnumName(ctx *EnumNameContext) interface{}

	// Visit a parse tree produced by CPP14Parser#enumSpecifier.
	VisitEnumSpecifier(ctx *EnumSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#enumHead.
	VisitEnumHead(ctx *EnumHeadContext) interface{}

	// Visit a parse tree produced by CPP14Parser#opaqueEnumDeclaration.
	VisitOpaqueEnumDeclaration(ctx *OpaqueEnumDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#enumKey.
	VisitEnumKey(ctx *EnumKeyContext) interface{}

	// Visit a parse tree produced by CPP14Parser#enumBase.
	VisitEnumBase(ctx *EnumBaseContext) interface{}

	// Visit a parse tree produced by CPP14Parser#enumeratorList.
	VisitEnumeratorList(ctx *EnumeratorListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#enumeratorDefinition.
	VisitEnumeratorDefinition(ctx *EnumeratorDefinitionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#enumerator.
	VisitEnumerator(ctx *EnumeratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#namespaceName.
	VisitNamespaceName(ctx *NamespaceNameContext) interface{}

	// Visit a parse tree produced by CPP14Parser#originalNamespaceName.
	VisitOriginalNamespaceName(ctx *OriginalNamespaceNameContext) interface{}

	// Visit a parse tree produced by CPP14Parser#namespaceDefinition.
	VisitNamespaceDefinition(ctx *NamespaceDefinitionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#namespaceAlias.
	VisitNamespaceAlias(ctx *NamespaceAliasContext) interface{}

	// Visit a parse tree produced by CPP14Parser#namespaceAliasDefinition.
	VisitNamespaceAliasDefinition(ctx *NamespaceAliasDefinitionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#qualifiedNamespaceSpecifier.
	VisitQualifiedNamespaceSpecifier(ctx *QualifiedNamespaceSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#usingDeclaration.
	VisitUsingDeclaration(ctx *UsingDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#usingDirective.
	VisitUsingDirective(ctx *UsingDirectiveContext) interface{}

	// Visit a parse tree produced by CPP14Parser#asmDefinition.
	VisitAsmDefinition(ctx *AsmDefinitionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#linkageSpecification.
	VisitLinkageSpecification(ctx *LinkageSpecificationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#attributeSpecifierSeq.
	VisitAttributeSpecifierSeq(ctx *AttributeSpecifierSeqContext) interface{}

	// Visit a parse tree produced by CPP14Parser#attributeSpecifier.
	VisitAttributeSpecifier(ctx *AttributeSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#alignmentSpecifier.
	VisitAlignmentSpecifier(ctx *AlignmentSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#attributeList.
	VisitAttributeList(ctx *AttributeListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#attribute.
	VisitAttribute(ctx *AttributeContext) interface{}

	// Visit a parse tree produced by CPP14Parser#attributeNamespace.
	VisitAttributeNamespace(ctx *AttributeNamespaceContext) interface{}

	// Visit a parse tree produced by CPP14Parser#attributeArgumentClause.
	VisitAttributeArgumentClause(ctx *AttributeArgumentClauseContext) interface{}

	// Visit a parse tree produced by CPP14Parser#balancedTokenSeq.
	VisitBalancedTokenSeq(ctx *BalancedTokenSeqContext) interface{}

	// Visit a parse tree produced by CPP14Parser#balancedToken.
	VisitBalancedToken(ctx *BalancedTokenContext) interface{}

	// Visit a parse tree produced by CPP14Parser#initDeclaratorList.
	VisitInitDeclaratorList(ctx *InitDeclaratorListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#initDeclarator.
	VisitInitDeclarator(ctx *InitDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#declarator.
	VisitDeclarator(ctx *DeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#pointerDeclarator.
	VisitPointerDeclarator(ctx *PointerDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#noPointerDeclarator.
	VisitNoPointerDeclarator(ctx *NoPointerDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#parametersAndQualifiers.
	VisitParametersAndQualifiers(ctx *ParametersAndQualifiersContext) interface{}

	// Visit a parse tree produced by CPP14Parser#trailingReturnType.
	VisitTrailingReturnType(ctx *TrailingReturnTypeContext) interface{}

	// Visit a parse tree produced by CPP14Parser#pointerOperator.
	VisitPointerOperator(ctx *PointerOperatorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#cvQualifierSeq.
	VisitCvQualifierSeq(ctx *CvQualifierSeqContext) interface{}

	// Visit a parse tree produced by CPP14Parser#cvQualifier.
	VisitCvQualifier(ctx *CvQualifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#refQualifier.
	VisitRefQualifier(ctx *RefQualifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#declaratorId.
	VisitDeclaratorId(ctx *DeclaratorIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#theTypeId.
	VisitTheTypeId(ctx *TheTypeIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#abstractDeclarator.
	VisitAbstractDeclarator(ctx *AbstractDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#pointerAbstractDeclarator.
	VisitPointerAbstractDeclarator(ctx *PointerAbstractDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#noPointerAbstractDeclarator.
	VisitNoPointerAbstractDeclarator(ctx *NoPointerAbstractDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#abstractPackDeclarator.
	VisitAbstractPackDeclarator(ctx *AbstractPackDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#noPointerAbstractPackDeclarator.
	VisitNoPointerAbstractPackDeclarator(ctx *NoPointerAbstractPackDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#parameterDeclarationClause.
	VisitParameterDeclarationClause(ctx *ParameterDeclarationClauseContext) interface{}

	// Visit a parse tree produced by CPP14Parser#parameterDeclarationList.
	VisitParameterDeclarationList(ctx *ParameterDeclarationListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#parameterDeclaration.
	VisitParameterDeclaration(ctx *ParameterDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#functionDefinition.
	VisitFunctionDefinition(ctx *FunctionDefinitionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#functionBody.
	VisitFunctionBody(ctx *FunctionBodyContext) interface{}

	// Visit a parse tree produced by CPP14Parser#initializer.
	VisitInitializer(ctx *InitializerContext) interface{}

	// Visit a parse tree produced by CPP14Parser#braceOrEqualInitializer.
	VisitBraceOrEqualInitializer(ctx *BraceOrEqualInitializerContext) interface{}

	// Visit a parse tree produced by CPP14Parser#initializerClause.
	VisitInitializerClause(ctx *InitializerClauseContext) interface{}

	// Visit a parse tree produced by CPP14Parser#initializerList.
	VisitInitializerList(ctx *InitializerListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#bracedInitList.
	VisitBracedInitList(ctx *BracedInitListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#className.
	VisitClassName(ctx *ClassNameContext) interface{}

	// Visit a parse tree produced by CPP14Parser#classSpecifier.
	VisitClassSpecifier(ctx *ClassSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#classHead.
	VisitClassHead(ctx *ClassHeadContext) interface{}

	// Visit a parse tree produced by CPP14Parser#classHeadName.
	VisitClassHeadName(ctx *ClassHeadNameContext) interface{}

	// Visit a parse tree produced by CPP14Parser#classVirtSpecifier.
	VisitClassVirtSpecifier(ctx *ClassVirtSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#classKey.
	VisitClassKey(ctx *ClassKeyContext) interface{}

	// Visit a parse tree produced by CPP14Parser#memberSpecification.
	VisitMemberSpecification(ctx *MemberSpecificationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#memberDeclaration.
	VisitMemberDeclaration(ctx *MemberDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#memberDeclaratorList.
	VisitMemberDeclaratorList(ctx *MemberDeclaratorListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#memberDeclarator.
	VisitMemberDeclarator(ctx *MemberDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#virtualSpecifierSeq.
	VisitVirtualSpecifierSeq(ctx *VirtualSpecifierSeqContext) interface{}

	// Visit a parse tree produced by CPP14Parser#virtualSpecifier.
	VisitVirtualSpecifier(ctx *VirtualSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#pureSpecifier.
	VisitPureSpecifier(ctx *PureSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#baseClause.
	VisitBaseClause(ctx *BaseClauseContext) interface{}

	// Visit a parse tree produced by CPP14Parser#baseSpecifierList.
	VisitBaseSpecifierList(ctx *BaseSpecifierListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#baseSpecifier.
	VisitBaseSpecifier(ctx *BaseSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#classOrDeclType.
	VisitClassOrDeclType(ctx *ClassOrDeclTypeContext) interface{}

	// Visit a parse tree produced by CPP14Parser#baseTypeSpecifier.
	VisitBaseTypeSpecifier(ctx *BaseTypeSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#accessSpecifier.
	VisitAccessSpecifier(ctx *AccessSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#conversionFunctionId.
	VisitConversionFunctionId(ctx *ConversionFunctionIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#conversionTypeId.
	VisitConversionTypeId(ctx *ConversionTypeIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#conversionDeclarator.
	VisitConversionDeclarator(ctx *ConversionDeclaratorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#constructorInitializer.
	VisitConstructorInitializer(ctx *ConstructorInitializerContext) interface{}

	// Visit a parse tree produced by CPP14Parser#memInitializerList.
	VisitMemInitializerList(ctx *MemInitializerListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#memInitializer.
	VisitMemInitializer(ctx *MemInitializerContext) interface{}

	// Visit a parse tree produced by CPP14Parser#memInitializerId.
	VisitMemInitializerId(ctx *MemInitializerIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#operatorFunctionId.
	VisitOperatorFunctionId(ctx *OperatorFunctionIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#literalOperatorId.
	VisitLiteralOperatorId(ctx *LiteralOperatorIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#templateDeclaration.
	VisitTemplateDeclaration(ctx *TemplateDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#templateParameterList.
	VisitTemplateParameterList(ctx *TemplateParameterListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#templateParameter.
	VisitTemplateParameter(ctx *TemplateParameterContext) interface{}

	// Visit a parse tree produced by CPP14Parser#typeParameter.
	VisitTypeParameter(ctx *TypeParameterContext) interface{}

	// Visit a parse tree produced by CPP14Parser#simpleTemplateId.
	VisitSimpleTemplateId(ctx *SimpleTemplateIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#templateId.
	VisitTemplateId(ctx *TemplateIdContext) interface{}

	// Visit a parse tree produced by CPP14Parser#templateName.
	VisitTemplateName(ctx *TemplateNameContext) interface{}

	// Visit a parse tree produced by CPP14Parser#templateArgumentList.
	VisitTemplateArgumentList(ctx *TemplateArgumentListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#templateArgument.
	VisitTemplateArgument(ctx *TemplateArgumentContext) interface{}

	// Visit a parse tree produced by CPP14Parser#typeNameSpecifier.
	VisitTypeNameSpecifier(ctx *TypeNameSpecifierContext) interface{}

	// Visit a parse tree produced by CPP14Parser#explicitInstantiation.
	VisitExplicitInstantiation(ctx *ExplicitInstantiationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#explicitSpecialization.
	VisitExplicitSpecialization(ctx *ExplicitSpecializationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#tryBlock.
	VisitTryBlock(ctx *TryBlockContext) interface{}

	// Visit a parse tree produced by CPP14Parser#functionTryBlock.
	VisitFunctionTryBlock(ctx *FunctionTryBlockContext) interface{}

	// Visit a parse tree produced by CPP14Parser#handlerSeq.
	VisitHandlerSeq(ctx *HandlerSeqContext) interface{}

	// Visit a parse tree produced by CPP14Parser#handler.
	VisitHandler(ctx *HandlerContext) interface{}

	// Visit a parse tree produced by CPP14Parser#exceptionDeclaration.
	VisitExceptionDeclaration(ctx *ExceptionDeclarationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#throwExpression.
	VisitThrowExpression(ctx *ThrowExpressionContext) interface{}

	// Visit a parse tree produced by CPP14Parser#exceptionSpecification.
	VisitExceptionSpecification(ctx *ExceptionSpecificationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#dynamicExceptionSpecification.
	VisitDynamicExceptionSpecification(ctx *DynamicExceptionSpecificationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#typeIdList.
	VisitTypeIdList(ctx *TypeIdListContext) interface{}

	// Visit a parse tree produced by CPP14Parser#noExceptSpecification.
	VisitNoExceptSpecification(ctx *NoExceptSpecificationContext) interface{}

	// Visit a parse tree produced by CPP14Parser#theOperator.
	VisitTheOperator(ctx *TheOperatorContext) interface{}

	// Visit a parse tree produced by CPP14Parser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}
}
