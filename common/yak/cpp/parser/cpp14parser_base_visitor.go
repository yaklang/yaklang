// Code generated from ./CPP14Parser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package cppparser // CPP14Parser
import "github.com/yaklang/antlr/v4"

type BaseCPP14ParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseCPP14ParserVisitor) VisitTranslationUnit(ctx *TranslationUnitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitPrimaryExpression(ctx *PrimaryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitIdExpression(ctx *IdExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitUnqualifiedId(ctx *UnqualifiedIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitQualifiedId(ctx *QualifiedIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNestedNameSpecifier(ctx *NestedNameSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitLambdaExpression(ctx *LambdaExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitLambdaIntroducer(ctx *LambdaIntroducerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitLambdaCapture(ctx *LambdaCaptureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitCaptureDefault(ctx *CaptureDefaultContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitCaptureList(ctx *CaptureListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitCapture(ctx *CaptureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitSimpleCapture(ctx *SimpleCaptureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitInitCapture(ctx *InitCaptureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitLambdaDeclarator(ctx *LambdaDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitPostfixExpression(ctx *PostfixExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTypeIdOfTheTypeId(ctx *TypeIdOfTheTypeIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitExpressionList(ctx *ExpressionListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitPseudoDestructorName(ctx *PseudoDestructorNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitUnaryExpression(ctx *UnaryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitUnaryOperator(ctx *UnaryOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNewExpression_(ctx *NewExpression_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNewPlacement(ctx *NewPlacementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNewTypeId(ctx *NewTypeIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNewDeclarator_(ctx *NewDeclarator_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNoPointerNewDeclarator(ctx *NoPointerNewDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNewInitializer_(ctx *NewInitializer_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitDeleteExpression(ctx *DeleteExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNoExceptExpression(ctx *NoExceptExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitCastExpression(ctx *CastExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitPointerMemberExpression(ctx *PointerMemberExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitShiftExpression(ctx *ShiftExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitShiftOperator(ctx *ShiftOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitRelationalExpression(ctx *RelationalExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitEqualityExpression(ctx *EqualityExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAndExpression(ctx *AndExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitExclusiveOrExpression(ctx *ExclusiveOrExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitInclusiveOrExpression(ctx *InclusiveOrExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitLogicalAndExpression(ctx *LogicalAndExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitLogicalOrExpression(ctx *LogicalOrExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitConditionalExpression(ctx *ConditionalExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAssignmentExpression(ctx *AssignmentExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAssignmentOperator(ctx *AssignmentOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitExpression(ctx *ExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitConstantExpression(ctx *ConstantExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitLabeledStatement(ctx *LabeledStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitExpressionStatement(ctx *ExpressionStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitCompoundStatement(ctx *CompoundStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitStatementSeq(ctx *StatementSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitSelectionStatement(ctx *SelectionStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitCondition(ctx *ConditionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitIterationStatement(ctx *IterationStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitForInitStatement(ctx *ForInitStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitForRangeDeclaration(ctx *ForRangeDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitForRangeInitializer(ctx *ForRangeInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitJumpStatement(ctx *JumpStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitDeclarationStatement(ctx *DeclarationStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitDeclarationSeq(ctx *DeclarationSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitDeclaration(ctx *DeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitBlockDeclaration(ctx *BlockDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAliasDeclaration(ctx *AliasDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitSimpleDeclaration(ctx *SimpleDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitStaticAssertDeclaration(ctx *StaticAssertDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitEmptyDeclaration_(ctx *EmptyDeclaration_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAttributeDeclaration(ctx *AttributeDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitDeclSpecifier(ctx *DeclSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitDeclSpecifierSeq(ctx *DeclSpecifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitStorageClassSpecifier(ctx *StorageClassSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitFunctionSpecifier(ctx *FunctionSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTypedefName(ctx *TypedefNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTypeSpecifier(ctx *TypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTrailingTypeSpecifier(ctx *TrailingTypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTypeSpecifierSeq(ctx *TypeSpecifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTrailingTypeSpecifierSeq(ctx *TrailingTypeSpecifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitSimpleTypeLengthModifier(ctx *SimpleTypeLengthModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitSimpleTypeSignednessModifier(ctx *SimpleTypeSignednessModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitSimpleTypeSpecifier(ctx *SimpleTypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTheTypeName(ctx *TheTypeNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitDecltypeSpecifier(ctx *DecltypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitElaboratedTypeSpecifier(ctx *ElaboratedTypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitEnumName(ctx *EnumNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitEnumSpecifier(ctx *EnumSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitEnumHead(ctx *EnumHeadContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitOpaqueEnumDeclaration(ctx *OpaqueEnumDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitEnumKey(ctx *EnumKeyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitEnumBase(ctx *EnumBaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitEnumeratorList(ctx *EnumeratorListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitEnumeratorDefinition(ctx *EnumeratorDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitEnumerator(ctx *EnumeratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNamespaceName(ctx *NamespaceNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitOriginalNamespaceName(ctx *OriginalNamespaceNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNamespaceDefinition(ctx *NamespaceDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNamespaceAlias(ctx *NamespaceAliasContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNamespaceAliasDefinition(ctx *NamespaceAliasDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitQualifiedNamespaceSpecifier(ctx *QualifiedNamespaceSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitUsingDeclaration(ctx *UsingDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitUsingDirective(ctx *UsingDirectiveContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAsmDefinition(ctx *AsmDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitLinkageSpecification(ctx *LinkageSpecificationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAttributeSpecifierSeq(ctx *AttributeSpecifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAttributeSpecifier(ctx *AttributeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAlignmentSpecifier(ctx *AlignmentSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAttributeList(ctx *AttributeListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAttribute(ctx *AttributeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAttributeNamespace(ctx *AttributeNamespaceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAttributeArgumentClause(ctx *AttributeArgumentClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitBalancedTokenSeq(ctx *BalancedTokenSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitBalancedToken(ctx *BalancedTokenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitInitDeclaratorList(ctx *InitDeclaratorListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitInitDeclarator(ctx *InitDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitDeclarator(ctx *DeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitPointerDeclarator(ctx *PointerDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNoPointerDeclarator(ctx *NoPointerDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitParametersAndQualifiers(ctx *ParametersAndQualifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTrailingReturnType(ctx *TrailingReturnTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitPointerOperator(ctx *PointerOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitCvQualifierSeq(ctx *CvQualifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitCvQualifier(ctx *CvQualifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitRefQualifier(ctx *RefQualifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitDeclaratorId(ctx *DeclaratorIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTheTypeId(ctx *TheTypeIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAbstractDeclarator(ctx *AbstractDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitPointerAbstractDeclarator(ctx *PointerAbstractDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNoPointerAbstractDeclarator(ctx *NoPointerAbstractDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAbstractPackDeclarator(ctx *AbstractPackDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNoPointerAbstractPackDeclarator(ctx *NoPointerAbstractPackDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitParameterDeclarationClause(ctx *ParameterDeclarationClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitParameterDeclarationList(ctx *ParameterDeclarationListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitParameterDeclaration(ctx *ParameterDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitFunctionDefinition(ctx *FunctionDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitFunctionBody(ctx *FunctionBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitInitializer(ctx *InitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitBraceOrEqualInitializer(ctx *BraceOrEqualInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitInitializerClause(ctx *InitializerClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitInitializerList(ctx *InitializerListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitBracedInitList(ctx *BracedInitListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitClassName(ctx *ClassNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitClassSpecifier(ctx *ClassSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitClassHead(ctx *ClassHeadContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitClassHeadName(ctx *ClassHeadNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitClassVirtSpecifier(ctx *ClassVirtSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitClassKey(ctx *ClassKeyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitMemberSpecification(ctx *MemberSpecificationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitMemberDeclaration(ctx *MemberDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitMemberDeclaratorList(ctx *MemberDeclaratorListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitMemberDeclarator(ctx *MemberDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitVirtualSpecifierSeq(ctx *VirtualSpecifierSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitVirtualSpecifier(ctx *VirtualSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitPureSpecifier(ctx *PureSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitBaseClause(ctx *BaseClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitBaseSpecifierList(ctx *BaseSpecifierListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitBaseSpecifier(ctx *BaseSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitClassOrDeclType(ctx *ClassOrDeclTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitBaseTypeSpecifier(ctx *BaseTypeSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitAccessSpecifier(ctx *AccessSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitConversionFunctionId(ctx *ConversionFunctionIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitConversionTypeId(ctx *ConversionTypeIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitConversionDeclarator(ctx *ConversionDeclaratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitConstructorInitializer(ctx *ConstructorInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitMemInitializerList(ctx *MemInitializerListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitMemInitializer(ctx *MemInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitMemInitializerId(ctx *MemInitializerIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitOperatorFunctionId(ctx *OperatorFunctionIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitLiteralOperatorId(ctx *LiteralOperatorIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTemplateDeclaration(ctx *TemplateDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTemplateParameterList(ctx *TemplateParameterListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTemplateParameter(ctx *TemplateParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTypeParameter(ctx *TypeParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitSimpleTemplateId(ctx *SimpleTemplateIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTemplateId(ctx *TemplateIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTemplateName(ctx *TemplateNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTemplateArgumentList(ctx *TemplateArgumentListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTemplateArgument(ctx *TemplateArgumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTypeNameSpecifier(ctx *TypeNameSpecifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitExplicitInstantiation(ctx *ExplicitInstantiationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitExplicitSpecialization(ctx *ExplicitSpecializationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTryBlock(ctx *TryBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitFunctionTryBlock(ctx *FunctionTryBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitHandlerSeq(ctx *HandlerSeqContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitHandler(ctx *HandlerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitExceptionDeclaration(ctx *ExceptionDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitThrowExpression(ctx *ThrowExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitExceptionSpecification(ctx *ExceptionSpecificationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitDynamicExceptionSpecification(ctx *DynamicExceptionSpecificationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTypeIdList(ctx *TypeIdListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitNoExceptSpecification(ctx *NoExceptSpecificationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitTheOperator(ctx *TheOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseCPP14ParserVisitor) VisitLiteral(ctx *LiteralContext) interface{} {
	return v.VisitChildren(ctx)
}
