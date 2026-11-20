// Code generated from ./CPPParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package cppparser // CPPParser
import "github.com/yaklang/antlr/v4"

type BaseCPPParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseCPPParserVisitor) VisitTranslationUnit(ctx *TranslationUnitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitPrimaryExpression(ctx *PrimaryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitFoldExpression(ctx *FoldExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitFoldOperator(ctx *FoldOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitIdExpression(ctx *IdExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitUnqualifiedId(ctx *UnqualifiedIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitQualifiedId(ctx *QualifiedIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNestedNameSpecifier(ctx *NestedNameSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitLambdaExpression(ctx *LambdaExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitLambdaIntroducer(ctx *LambdaIntroducerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitLambdaCapture(ctx *LambdaCaptureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitCaptureDefault(ctx *CaptureDefaultContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitCaptureList(ctx *CaptureListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitCapture(ctx *CaptureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitSimpleCapture(ctx *SimpleCaptureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitInitCapture(ctx *InitCaptureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitLambdaDeclarator(ctx *LambdaDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitPostfixExpression(ctx *PostfixExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTypeIdOfTheTypeId(ctx *TypeIdOfTheTypeIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitExpressionList(ctx *ExpressionListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitPseudoDestructorName(ctx *PseudoDestructorNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitUnaryExpression(ctx *UnaryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitUnaryOperator(ctx *UnaryOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNewExpression_(ctx *NewExpression_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNewPlacement(ctx *NewPlacementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNewTypeId(ctx *NewTypeIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNewDeclarator_(ctx *NewDeclarator_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNoPointerNewDeclarator(ctx *NoPointerNewDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNewInitializer_(ctx *NewInitializer_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitDeleteExpression(ctx *DeleteExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNoExceptExpression(ctx *NoExceptExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitCastExpression(ctx *CastExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitPointerMemberExpression(ctx *PointerMemberExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitShiftExpression(ctx *ShiftExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitShiftOperator(ctx *ShiftOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitRelationalExpression(ctx *RelationalExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitEqualityExpression(ctx *EqualityExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAndExpression(ctx *AndExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitExclusiveOrExpression(ctx *ExclusiveOrExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitInclusiveOrExpression(ctx *InclusiveOrExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitLogicalAndExpression(ctx *LogicalAndExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitLogicalOrExpression(ctx *LogicalOrExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitConditionalExpression(ctx *ConditionalExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAssignmentExpression(ctx *AssignmentExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAssignmentOperator(ctx *AssignmentOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitExpression(ctx *ExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitConstantExpression(ctx *ConstantExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitLabeledStatement(ctx *LabeledStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitExpressionStatement(ctx *ExpressionStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitCompoundStatement(ctx *CompoundStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitStatementSeq(ctx *StatementSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitSelectionStatement(ctx *SelectionStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitCondition(ctx *ConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitIterationStatement(ctx *IterationStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitForInitStatement(ctx *ForInitStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitForRangeDeclaration(ctx *ForRangeDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitForRangeInitializer(ctx *ForRangeInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitJumpStatement(ctx *JumpStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitDeclarationStatement(ctx *DeclarationStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitDeclarationSeq(ctx *DeclarationSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitDeclaration(ctx *DeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitBlockDeclaration(ctx *BlockDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAliasDeclaration(ctx *AliasDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitSimpleDeclaration(ctx *SimpleDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitIdentifierList(ctx *IdentifierListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitStaticAssertDeclaration(ctx *StaticAssertDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitEmptyDeclaration_(ctx *EmptyDeclaration_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAttributeDeclaration(ctx *AttributeDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitDeclSpecifier(ctx *DeclSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitDeclSpecifierSeq(ctx *DeclSpecifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitStorageClassSpecifier(ctx *StorageClassSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitFunctionSpecifier(ctx *FunctionSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTypedefName(ctx *TypedefNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTypeSpecifier(ctx *TypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTrailingTypeSpecifier(ctx *TrailingTypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTypeSpecifierSeq(ctx *TypeSpecifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTrailingTypeSpecifierSeq(ctx *TrailingTypeSpecifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitSimpleTypeLengthModifier(ctx *SimpleTypeLengthModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitSimpleTypeSignednessModifier(ctx *SimpleTypeSignednessModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitSimpleTypeSpecifier(ctx *SimpleTypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTheTypeName(ctx *TheTypeNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitDecltypeSpecifier(ctx *DecltypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitElaboratedTypeSpecifier(ctx *ElaboratedTypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitEnumName(ctx *EnumNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitEnumSpecifier(ctx *EnumSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitEnumHead(ctx *EnumHeadContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitOpaqueEnumDeclaration(ctx *OpaqueEnumDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitEnumKey(ctx *EnumKeyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitEnumBase(ctx *EnumBaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitEnumeratorList(ctx *EnumeratorListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitEnumeratorDefinition(ctx *EnumeratorDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitEnumerator(ctx *EnumeratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNamespaceName(ctx *NamespaceNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitOriginalNamespaceName(ctx *OriginalNamespaceNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNamespaceDefinition(ctx *NamespaceDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNamespaceAlias(ctx *NamespaceAliasContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNamespaceAliasDefinition(ctx *NamespaceAliasDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitQualifiedNamespaceSpecifier(ctx *QualifiedNamespaceSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitUsingDeclaration(ctx *UsingDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitUsingDirective(ctx *UsingDirectiveContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAsmDefinition(ctx *AsmDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitLinkageSpecification(ctx *LinkageSpecificationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAttributeSpecifierSeq(ctx *AttributeSpecifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAttributeSpecifier(ctx *AttributeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAlignmentSpecifier(ctx *AlignmentSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAttributeList(ctx *AttributeListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAttribute(ctx *AttributeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAttributeNamespace(ctx *AttributeNamespaceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAttributeArgumentClause(ctx *AttributeArgumentClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitBalancedTokenSeq(ctx *BalancedTokenSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitBalancedToken(ctx *BalancedTokenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitInitDeclaratorList(ctx *InitDeclaratorListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitInitDeclarator(ctx *InitDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitDeclarator(ctx *DeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitPointerDeclarator(ctx *PointerDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNoPointerDeclarator(ctx *NoPointerDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitParametersAndQualifiers(ctx *ParametersAndQualifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTrailingReturnType(ctx *TrailingReturnTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitPointerOperator(ctx *PointerOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitCvQualifierSeq(ctx *CvQualifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitCvQualifier(ctx *CvQualifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitRefQualifier(ctx *RefQualifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitDeclaratorId(ctx *DeclaratorIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTheTypeId(ctx *TheTypeIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAbstractDeclarator(ctx *AbstractDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitPointerAbstractDeclarator(ctx *PointerAbstractDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNoPointerAbstractDeclarator(ctx *NoPointerAbstractDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAbstractPackDeclarator(ctx *AbstractPackDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNoPointerAbstractPackDeclarator(ctx *NoPointerAbstractPackDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitParameterDeclarationClause(ctx *ParameterDeclarationClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitParameterDeclarationList(ctx *ParameterDeclarationListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitParameterDeclaration(ctx *ParameterDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitFunctionDefinition(ctx *FunctionDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitFunctionBody(ctx *FunctionBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitInitializer(ctx *InitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitBraceOrEqualInitializer(ctx *BraceOrEqualInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitInitializerClause(ctx *InitializerClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitInitializerList(ctx *InitializerListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitBracedInitList(ctx *BracedInitListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitClassName(ctx *ClassNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitClassSpecifier(ctx *ClassSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitClassHead(ctx *ClassHeadContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitClassHeadName(ctx *ClassHeadNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitClassVirtSpecifier(ctx *ClassVirtSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitClassKey(ctx *ClassKeyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitMemberSpecification(ctx *MemberSpecificationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitMemberDeclaration(ctx *MemberDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitMemberDeclaratorList(ctx *MemberDeclaratorListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitMemberDeclarator(ctx *MemberDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitVirtualSpecifierSeq(ctx *VirtualSpecifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitVirtualSpecifier(ctx *VirtualSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitPureSpecifier(ctx *PureSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitBaseClause(ctx *BaseClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitBaseSpecifierList(ctx *BaseSpecifierListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitBaseSpecifier(ctx *BaseSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitClassOrDeclType(ctx *ClassOrDeclTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitBaseTypeSpecifier(ctx *BaseTypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitAccessSpecifier(ctx *AccessSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitConversionFunctionId(ctx *ConversionFunctionIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitConversionTypeId(ctx *ConversionTypeIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitConversionDeclarator(ctx *ConversionDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitConstructorInitializer(ctx *ConstructorInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitMemInitializerList(ctx *MemInitializerListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitMemInitializer(ctx *MemInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitMemInitializerId(ctx *MemInitializerIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitOperatorFunctionId(ctx *OperatorFunctionIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitLiteralOperatorId(ctx *LiteralOperatorIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTemplateDeclaration(ctx *TemplateDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTemplateParameterList(ctx *TemplateParameterListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTemplateParameter(ctx *TemplateParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTypeParameter(ctx *TypeParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitSimpleTemplateId(ctx *SimpleTemplateIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTemplateId(ctx *TemplateIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTemplateName(ctx *TemplateNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTemplateArgumentList(ctx *TemplateArgumentListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTemplateArgument(ctx *TemplateArgumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTypeNameSpecifier(ctx *TypeNameSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitExplicitInstantiation(ctx *ExplicitInstantiationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitExplicitSpecialization(ctx *ExplicitSpecializationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTryBlock(ctx *TryBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitFunctionTryBlock(ctx *FunctionTryBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitHandlerSeq(ctx *HandlerSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitHandler(ctx *HandlerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitExceptionDeclaration(ctx *ExceptionDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitThrowExpression(ctx *ThrowExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitExceptionSpecification(ctx *ExceptionSpecificationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitDynamicExceptionSpecification(ctx *DynamicExceptionSpecificationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTypeIdList(ctx *TypeIdListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitNoExceptSpecification(ctx *NoExceptSpecificationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitTheOperator(ctx *TheOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPPParserVisitor) VisitLiteral(ctx *LiteralContext) interface{} {
	return v.VisitChildren(ctx)
}
