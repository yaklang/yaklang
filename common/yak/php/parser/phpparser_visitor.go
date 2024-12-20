// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package phpparser // PHPParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by PHPParser.
type PHPParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by PHPParser#htmlDocument.
	VisitHtmlDocument(ctx *HtmlDocumentContext) interface{}

	// Visit a parse tree produced by PHPParser#htmlDocumentElement.
	VisitHtmlDocumentElement(ctx *HtmlDocumentElementContext) interface{}

	// Visit a parse tree produced by PHPParser#inlineHtml.
	VisitInlineHtml(ctx *InlineHtmlContext) interface{}

	// Visit a parse tree produced by PHPParser#htmlElement.
	VisitHtmlElement(ctx *HtmlElementContext) interface{}

	// Visit a parse tree produced by PHPParser#scriptText.
	VisitScriptText(ctx *ScriptTextContext) interface{}

	// Visit a parse tree produced by PHPParser#phpBlock.
	VisitPhpBlock(ctx *PhpBlockContext) interface{}

	// Visit a parse tree produced by PHPParser#importStatement.
	VisitImportStatement(ctx *ImportStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#topStatement.
	VisitTopStatement(ctx *TopStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#useDeclaration.
	VisitUseDeclaration(ctx *UseDeclarationContext) interface{}

	// Visit a parse tree produced by PHPParser#useDeclarationContentList.
	VisitUseDeclarationContentList(ctx *UseDeclarationContentListContext) interface{}

	// Visit a parse tree produced by PHPParser#namespacePath.
	VisitNamespacePath(ctx *NamespacePathContext) interface{}

	// Visit a parse tree produced by PHPParser#namespaceDeclaration.
	VisitNamespaceDeclaration(ctx *NamespaceDeclarationContext) interface{}

	// Visit a parse tree produced by PHPParser#namespaceStatement.
	VisitNamespaceStatement(ctx *NamespaceStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#functionDeclaration.
	VisitFunctionDeclaration(ctx *FunctionDeclarationContext) interface{}

	// Visit a parse tree produced by PHPParser#classDeclaration.
	VisitClassDeclaration(ctx *ClassDeclarationContext) interface{}

	// Visit a parse tree produced by PHPParser#classEntryType.
	VisitClassEntryType(ctx *ClassEntryTypeContext) interface{}

	// Visit a parse tree produced by PHPParser#interfaceList.
	VisitInterfaceList(ctx *InterfaceListContext) interface{}

	// Visit a parse tree produced by PHPParser#typeParameterList.
	VisitTypeParameterList(ctx *TypeParameterListContext) interface{}

	// Visit a parse tree produced by PHPParser#typeParameterWithDefaultsList.
	VisitTypeParameterWithDefaultsList(ctx *TypeParameterWithDefaultsListContext) interface{}

	// Visit a parse tree produced by PHPParser#typeParameterDecl.
	VisitTypeParameterDecl(ctx *TypeParameterDeclContext) interface{}

	// Visit a parse tree produced by PHPParser#typeParameterWithDefaultDecl.
	VisitTypeParameterWithDefaultDecl(ctx *TypeParameterWithDefaultDeclContext) interface{}

	// Visit a parse tree produced by PHPParser#attributes.
	VisitAttributes(ctx *AttributesContext) interface{}

	// Visit a parse tree produced by PHPParser#attributeGroup.
	VisitAttributeGroup(ctx *AttributeGroupContext) interface{}

	// Visit a parse tree produced by PHPParser#attribute.
	VisitAttribute(ctx *AttributeContext) interface{}

	// Visit a parse tree produced by PHPParser#innerStatementList.
	VisitInnerStatementList(ctx *InnerStatementListContext) interface{}

	// Visit a parse tree produced by PHPParser#innerStatement.
	VisitInnerStatement(ctx *InnerStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#labelStatement.
	VisitLabelStatement(ctx *LabelStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by PHPParser#emptyStatement_.
	VisitEmptyStatement_(ctx *EmptyStatement_Context) interface{}

	// Visit a parse tree produced by PHPParser#blockStatement.
	VisitBlockStatement(ctx *BlockStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#ifStatement.
	VisitIfStatement(ctx *IfStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#elseIfStatement.
	VisitElseIfStatement(ctx *ElseIfStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#elseIfColonStatement.
	VisitElseIfColonStatement(ctx *ElseIfColonStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#elseStatement.
	VisitElseStatement(ctx *ElseStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#elseColonStatement.
	VisitElseColonStatement(ctx *ElseColonStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#whileStatement.
	VisitWhileStatement(ctx *WhileStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#doWhileStatement.
	VisitDoWhileStatement(ctx *DoWhileStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#forStatement.
	VisitForStatement(ctx *ForStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#forInit.
	VisitForInit(ctx *ForInitContext) interface{}

	// Visit a parse tree produced by PHPParser#forUpdate.
	VisitForUpdate(ctx *ForUpdateContext) interface{}

	// Visit a parse tree produced by PHPParser#switchStatement.
	VisitSwitchStatement(ctx *SwitchStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#switchCaseBlock.
	VisitSwitchCaseBlock(ctx *SwitchCaseBlockContext) interface{}

	// Visit a parse tree produced by PHPParser#switchDefaultBlock.
	VisitSwitchDefaultBlock(ctx *SwitchDefaultBlockContext) interface{}

	// Visit a parse tree produced by PHPParser#switchBlock.
	VisitSwitchBlock(ctx *SwitchBlockContext) interface{}

	// Visit a parse tree produced by PHPParser#breakStatement.
	VisitBreakStatement(ctx *BreakStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#continueStatement.
	VisitContinueStatement(ctx *ContinueStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#returnStatement.
	VisitReturnStatement(ctx *ReturnStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#expressionStatement.
	VisitExpressionStatement(ctx *ExpressionStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#unsetStatement.
	VisitUnsetStatement(ctx *UnsetStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#foreachStatement.
	VisitForeachStatement(ctx *ForeachStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#tryCatchFinally.
	VisitTryCatchFinally(ctx *TryCatchFinallyContext) interface{}

	// Visit a parse tree produced by PHPParser#catchClause.
	VisitCatchClause(ctx *CatchClauseContext) interface{}

	// Visit a parse tree produced by PHPParser#finallyStatement.
	VisitFinallyStatement(ctx *FinallyStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#throwStatement.
	VisitThrowStatement(ctx *ThrowStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#gotoStatement.
	VisitGotoStatement(ctx *GotoStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#declareStatement.
	VisitDeclareStatement(ctx *DeclareStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#inlineHtmlStatement.
	VisitInlineHtmlStatement(ctx *InlineHtmlStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#declareList.
	VisitDeclareList(ctx *DeclareListContext) interface{}

	// Visit a parse tree produced by PHPParser#directive.
	VisitDirective(ctx *DirectiveContext) interface{}

	// Visit a parse tree produced by PHPParser#formalParameterList.
	VisitFormalParameterList(ctx *FormalParameterListContext) interface{}

	// Visit a parse tree produced by PHPParser#formalParameter.
	VisitFormalParameter(ctx *FormalParameterContext) interface{}

	// Visit a parse tree produced by PHPParser#typeHint.
	VisitTypeHint(ctx *TypeHintContext) interface{}

	// Visit a parse tree produced by PHPParser#globalStatement.
	VisitGlobalStatement(ctx *GlobalStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#echoStatement.
	VisitEchoStatement(ctx *EchoStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#staticVariableStatement.
	VisitStaticVariableStatement(ctx *StaticVariableStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#TraitUse.
	VisitTraitUse(ctx *TraitUseContext) interface{}

	// Visit a parse tree produced by PHPParser#propertyModifiersVariable.
	VisitPropertyModifiersVariable(ctx *PropertyModifiersVariableContext) interface{}

	// Visit a parse tree produced by PHPParser#Const.
	VisitConst(ctx *ConstContext) interface{}

	// Visit a parse tree produced by PHPParser#Function.
	VisitFunction(ctx *FunctionContext) interface{}

	// Visit a parse tree produced by PHPParser#traitAdaptations.
	VisitTraitAdaptations(ctx *TraitAdaptationsContext) interface{}

	// Visit a parse tree produced by PHPParser#traitAdaptationStatement.
	VisitTraitAdaptationStatement(ctx *TraitAdaptationStatementContext) interface{}

	// Visit a parse tree produced by PHPParser#traitPrecedence.
	VisitTraitPrecedence(ctx *TraitPrecedenceContext) interface{}

	// Visit a parse tree produced by PHPParser#traitAlias.
	VisitTraitAlias(ctx *TraitAliasContext) interface{}

	// Visit a parse tree produced by PHPParser#traitMethodReference.
	VisitTraitMethodReference(ctx *TraitMethodReferenceContext) interface{}

	// Visit a parse tree produced by PHPParser#baseCtorCall.
	VisitBaseCtorCall(ctx *BaseCtorCallContext) interface{}

	// Visit a parse tree produced by PHPParser#returnTypeDecl.
	VisitReturnTypeDecl(ctx *ReturnTypeDeclContext) interface{}

	// Visit a parse tree produced by PHPParser#methodBody.
	VisitMethodBody(ctx *MethodBodyContext) interface{}

	// Visit a parse tree produced by PHPParser#propertyModifiers.
	VisitPropertyModifiers(ctx *PropertyModifiersContext) interface{}

	// Visit a parse tree produced by PHPParser#memberModifiers.
	VisitMemberModifiers(ctx *MemberModifiersContext) interface{}

	// Visit a parse tree produced by PHPParser#variableInitializer.
	VisitVariableInitializer(ctx *VariableInitializerContext) interface{}

	// Visit a parse tree produced by PHPParser#identifierInitializer.
	VisitIdentifierInitializer(ctx *IdentifierInitializerContext) interface{}

	// Visit a parse tree produced by PHPParser#globalConstantDeclaration.
	VisitGlobalConstantDeclaration(ctx *GlobalConstantDeclarationContext) interface{}

	// Visit a parse tree produced by PHPParser#enumDeclaration.
	VisitEnumDeclaration(ctx *EnumDeclarationContext) interface{}

	// Visit a parse tree produced by PHPParser#enumItem.
	VisitEnumItem(ctx *EnumItemContext) interface{}

	// Visit a parse tree produced by PHPParser#expressionList.
	VisitExpressionList(ctx *ExpressionListContext) interface{}

	// Visit a parse tree produced by PHPParser#parentheses.
	VisitParentheses(ctx *ParenthesesContext) interface{}

	// Visit a parse tree produced by PHPParser#fullyQualifiedNamespaceExpr.
	VisitFullyQualifiedNamespaceExpr(ctx *FullyQualifiedNamespaceExprContext) interface{}

	// Visit a parse tree produced by PHPParser#staticClassExpr.
	VisitStaticClassExpr(ctx *StaticClassExprContext) interface{}

	// Visit a parse tree produced by PHPParser#staticClassExprFunctionMember.
	VisitStaticClassExprFunctionMember(ctx *StaticClassExprFunctionMemberContext) interface{}

	// Visit a parse tree produced by PHPParser#staticClassExprVariableMember.
	VisitStaticClassExprVariableMember(ctx *StaticClassExprVariableMemberContext) interface{}

	// Visit a parse tree produced by PHPParser#staticClass.
	VisitStaticClass(ctx *StaticClassContext) interface{}

	// Visit a parse tree produced by PHPParser#memberCallKey.
	VisitMemberCallKey(ctx *MemberCallKeyContext) interface{}

	// Visit a parse tree produced by PHPParser#indexMemberCallKey.
	VisitIndexMemberCallKey(ctx *IndexMemberCallKeyContext) interface{}

	// Visit a parse tree produced by PHPParser#SpecialWordExpression.
	VisitSpecialWordExpression(ctx *SpecialWordExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#ShortQualifiedNameExpression.
	VisitShortQualifiedNameExpression(ctx *ShortQualifiedNameExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#ArrayCreationExpression.
	VisitArrayCreationExpression(ctx *ArrayCreationExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#BackQuoteStringExpression.
	VisitBackQuoteStringExpression(ctx *BackQuoteStringExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#MemberCallExpression.
	VisitMemberCallExpression(ctx *MemberCallExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#ArrayCreationUnpackExpression.
	VisitArrayCreationUnpackExpression(ctx *ArrayCreationUnpackExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#KeywordNewExpression.
	VisitKeywordNewExpression(ctx *KeywordNewExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#MatchExpression.
	VisitMatchExpression(ctx *MatchExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#FunctionCallExpression.
	VisitFunctionCallExpression(ctx *FunctionCallExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#LogicalExpression.
	VisitLogicalExpression(ctx *LogicalExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#PrintExpression.
	VisitPrintExpression(ctx *PrintExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#PostfixIncDecExpression.
	VisitPostfixIncDecExpression(ctx *PostfixIncDecExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#IncludeExpression.
	VisitIncludeExpression(ctx *IncludeExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#IndexCallExpression.
	VisitIndexCallExpression(ctx *IndexCallExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#CloneExpression.
	VisitCloneExpression(ctx *CloneExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#StaticClassMemberCallAssignmentExpression.
	VisitStaticClassMemberCallAssignmentExpression(ctx *StaticClassMemberCallAssignmentExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#UnaryOperatorExpression.
	VisitUnaryOperatorExpression(ctx *UnaryOperatorExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#ParenthesisExpression.
	VisitParenthesisExpression(ctx *ParenthesisExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#SpaceshipExpression.
	VisitSpaceshipExpression(ctx *SpaceshipExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#ConditionalExpression.
	VisitConditionalExpression(ctx *ConditionalExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#TemplateExpression.
	VisitTemplateExpression(ctx *TemplateExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#VariableExpression.
	VisitVariableExpression(ctx *VariableExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#NullCoalescingExpression.
	VisitNullCoalescingExpression(ctx *NullCoalescingExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#DefinedOrScanDefinedExpression.
	VisitDefinedOrScanDefinedExpression(ctx *DefinedOrScanDefinedExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#ArithmeticExpression.
	VisitArithmeticExpression(ctx *ArithmeticExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#ScalarExpression.
	VisitScalarExpression(ctx *ScalarExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#PrefixIncDecExpression.
	VisitPrefixIncDecExpression(ctx *PrefixIncDecExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#ComparisonExpression.
	VisitComparisonExpression(ctx *ComparisonExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#ParentExpression.
	VisitParentExpression(ctx *ParentExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#IndexLegacyCallExpression.
	VisitIndexLegacyCallExpression(ctx *IndexLegacyCallExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#OrdinaryAssignmentExpression.
	VisitOrdinaryAssignmentExpression(ctx *OrdinaryAssignmentExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#CastExpression.
	VisitCastExpression(ctx *CastExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#InstanceOfExpression.
	VisitInstanceOfExpression(ctx *InstanceOfExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#LambdaFunctionExpression.
	VisitLambdaFunctionExpression(ctx *LambdaFunctionExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#BitwiseExpression.
	VisitBitwiseExpression(ctx *BitwiseExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#FullyQualifiedNamespaceExpression.
	VisitFullyQualifiedNamespaceExpression(ctx *FullyQualifiedNamespaceExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#StaticClassAccessExpression.
	VisitStaticClassAccessExpression(ctx *StaticClassAccessExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#MemberFunction.
	VisitMemberFunction(ctx *MemberFunctionContext) interface{}

	// Visit a parse tree produced by PHPParser#IndexLegacyCallVariable.
	VisitIndexLegacyCallVariable(ctx *IndexLegacyCallVariableContext) interface{}

	// Visit a parse tree produced by PHPParser#IndexVariable.
	VisitIndexVariable(ctx *IndexVariableContext) interface{}

	// Visit a parse tree produced by PHPParser#CustomVariable.
	VisitCustomVariable(ctx *CustomVariableContext) interface{}

	// Visit a parse tree produced by PHPParser#MemberVariable.
	VisitMemberVariable(ctx *MemberVariableContext) interface{}

	// Visit a parse tree produced by PHPParser#defineExpr.
	VisitDefineExpr(ctx *DefineExprContext) interface{}

	// Visit a parse tree produced by PHPParser#NormalVariable.
	VisitNormalVariable(ctx *NormalVariableContext) interface{}

	// Visit a parse tree produced by PHPParser#DynamicVariable.
	VisitDynamicVariable(ctx *DynamicVariableContext) interface{}

	// Visit a parse tree produced by PHPParser#MemberCallVariable.
	VisitMemberCallVariable(ctx *MemberCallVariableContext) interface{}

	// Visit a parse tree produced by PHPParser#include.
	VisitInclude(ctx *IncludeContext) interface{}

	// Visit a parse tree produced by PHPParser#leftArrayCreation.
	VisitLeftArrayCreation(ctx *LeftArrayCreationContext) interface{}

	// Visit a parse tree produced by PHPParser#assignable.
	VisitAssignable(ctx *AssignableContext) interface{}

	// Visit a parse tree produced by PHPParser#arrayCreation.
	VisitArrayCreation(ctx *ArrayCreationContext) interface{}

	// Visit a parse tree produced by PHPParser#arrayDestructuring.
	VisitArrayDestructuring(ctx *ArrayDestructuringContext) interface{}

	// Visit a parse tree produced by PHPParser#indexedDestructItem.
	VisitIndexedDestructItem(ctx *IndexedDestructItemContext) interface{}

	// Visit a parse tree produced by PHPParser#keyedDestructItem.
	VisitKeyedDestructItem(ctx *KeyedDestructItemContext) interface{}

	// Visit a parse tree produced by PHPParser#lambdaFunctionExpr.
	VisitLambdaFunctionExpr(ctx *LambdaFunctionExprContext) interface{}

	// Visit a parse tree produced by PHPParser#matchExpr.
	VisitMatchExpr(ctx *MatchExprContext) interface{}

	// Visit a parse tree produced by PHPParser#matchItem.
	VisitMatchItem(ctx *MatchItemContext) interface{}

	// Visit a parse tree produced by PHPParser#newExpr.
	VisitNewExpr(ctx *NewExprContext) interface{}

	// Visit a parse tree produced by PHPParser#assignmentOperator.
	VisitAssignmentOperator(ctx *AssignmentOperatorContext) interface{}

	// Visit a parse tree produced by PHPParser#yieldExpression.
	VisitYieldExpression(ctx *YieldExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#arrayItemList.
	VisitArrayItemList(ctx *ArrayItemListContext) interface{}

	// Visit a parse tree produced by PHPParser#arrayItem.
	VisitArrayItem(ctx *ArrayItemContext) interface{}

	// Visit a parse tree produced by PHPParser#lambdaFunctionUseVars.
	VisitLambdaFunctionUseVars(ctx *LambdaFunctionUseVarsContext) interface{}

	// Visit a parse tree produced by PHPParser#lambdaFunctionUseVar.
	VisitLambdaFunctionUseVar(ctx *LambdaFunctionUseVarContext) interface{}

	// Visit a parse tree produced by PHPParser#qualifiedStaticTypeRef.
	VisitQualifiedStaticTypeRef(ctx *QualifiedStaticTypeRefContext) interface{}

	// Visit a parse tree produced by PHPParser#typeRef.
	VisitTypeRef(ctx *TypeRefContext) interface{}

	// Visit a parse tree produced by PHPParser#anonymousClass.
	VisitAnonymousClass(ctx *AnonymousClassContext) interface{}

	// Visit a parse tree produced by PHPParser#indirectTypeRef.
	VisitIndirectTypeRef(ctx *IndirectTypeRefContext) interface{}

	// Visit a parse tree produced by PHPParser#qualifiedNamespaceName.
	VisitQualifiedNamespaceName(ctx *QualifiedNamespaceNameContext) interface{}

	// Visit a parse tree produced by PHPParser#NamespaceIdentifier.
	VisitNamespaceIdentifier(ctx *NamespaceIdentifierContext) interface{}

	// Visit a parse tree produced by PHPParser#NamespaceListNameTail.
	VisitNamespaceListNameTail(ctx *NamespaceListNameTailContext) interface{}

	// Visit a parse tree produced by PHPParser#namespaceNameTail.
	VisitNamespaceNameTail(ctx *NamespaceNameTailContext) interface{}

	// Visit a parse tree produced by PHPParser#qualifiedNamespaceNameList.
	VisitQualifiedNamespaceNameList(ctx *QualifiedNamespaceNameListContext) interface{}

	// Visit a parse tree produced by PHPParser#arguments.
	VisitArguments(ctx *ArgumentsContext) interface{}

	// Visit a parse tree produced by PHPParser#actualArgument.
	VisitActualArgument(ctx *ActualArgumentContext) interface{}

	// Visit a parse tree produced by PHPParser#argumentName.
	VisitArgumentName(ctx *ArgumentNameContext) interface{}

	// Visit a parse tree produced by PHPParser#ConstantStringitializer.
	VisitConstantStringitializer(ctx *ConstantStringitializerContext) interface{}

	// Visit a parse tree produced by PHPParser#ArrayInitializer.
	VisitArrayInitializer(ctx *ArrayInitializerContext) interface{}

	// Visit a parse tree produced by PHPParser#Unitializer.
	VisitUnitializer(ctx *UnitializerContext) interface{}

	// Visit a parse tree produced by PHPParser#Expressionitializer.
	VisitExpressionitializer(ctx *ExpressionitializerContext) interface{}

	// Visit a parse tree produced by PHPParser#constantString.
	VisitConstantString(ctx *ConstantStringContext) interface{}

	// Visit a parse tree produced by PHPParser#constant.
	VisitConstant(ctx *ConstantContext) interface{}

	// Visit a parse tree produced by PHPParser#literalConstant.
	VisitLiteralConstant(ctx *LiteralConstantContext) interface{}

	// Visit a parse tree produced by PHPParser#numericConstant.
	VisitNumericConstant(ctx *NumericConstantContext) interface{}

	// Visit a parse tree produced by PHPParser#classConstant.
	VisitClassConstant(ctx *ClassConstantContext) interface{}

	// Visit a parse tree produced by PHPParser#stringConstant.
	VisitStringConstant(ctx *StringConstantContext) interface{}

	// Visit a parse tree produced by PHPParser#string.
	VisitString(ctx *StringContext) interface{}

	// Visit a parse tree produced by PHPParser#hereDocContent.
	VisitHereDocContent(ctx *HereDocContentContext) interface{}

	// Visit a parse tree produced by PHPParser#interpolatedStringPart.
	VisitInterpolatedStringPart(ctx *InterpolatedStringPartContext) interface{}

	// Visit a parse tree produced by PHPParser#chainList.
	VisitChainList(ctx *ChainListContext) interface{}

	// Visit a parse tree produced by PHPParser#chain.
	VisitChain(ctx *ChainContext) interface{}

	// Visit a parse tree produced by PHPParser#chainOrigin.
	VisitChainOrigin(ctx *ChainOriginContext) interface{}

	// Visit a parse tree produced by PHPParser#memberAccess.
	VisitMemberAccess(ctx *MemberAccessContext) interface{}

	// Visit a parse tree produced by PHPParser#functionCall.
	VisitFunctionCall(ctx *FunctionCallContext) interface{}

	// Visit a parse tree produced by PHPParser#functionCallName.
	VisitFunctionCallName(ctx *FunctionCallNameContext) interface{}

	// Visit a parse tree produced by PHPParser#actualArguments.
	VisitActualArguments(ctx *ActualArgumentsContext) interface{}

	// Visit a parse tree produced by PHPParser#chainBase.
	VisitChainBase(ctx *ChainBaseContext) interface{}

	// Visit a parse tree produced by PHPParser#keyedFieldName.
	VisitKeyedFieldName(ctx *KeyedFieldNameContext) interface{}

	// Visit a parse tree produced by PHPParser#keyedSimpleFieldName.
	VisitKeyedSimpleFieldName(ctx *KeyedSimpleFieldNameContext) interface{}

	// Visit a parse tree produced by PHPParser#keyedVariable.
	VisitKeyedVariable(ctx *KeyedVariableContext) interface{}

	// Visit a parse tree produced by PHPParser#squareCurlyExpression.
	VisitSquareCurlyExpression(ctx *SquareCurlyExpressionContext) interface{}

	// Visit a parse tree produced by PHPParser#assignmentList.
	VisitAssignmentList(ctx *AssignmentListContext) interface{}

	// Visit a parse tree produced by PHPParser#assignmentListElement.
	VisitAssignmentListElement(ctx *AssignmentListElementContext) interface{}

	// Visit a parse tree produced by PHPParser#modifier.
	VisitModifier(ctx *ModifierContext) interface{}

	// Visit a parse tree produced by PHPParser#identifier.
	VisitIdentifier(ctx *IdentifierContext) interface{}

	// Visit a parse tree produced by PHPParser#key.
	VisitKey(ctx *KeyContext) interface{}

	// Visit a parse tree produced by PHPParser#memberModifier.
	VisitMemberModifier(ctx *MemberModifierContext) interface{}

	// Visit a parse tree produced by PHPParser#magicConstant.
	VisitMagicConstant(ctx *MagicConstantContext) interface{}

	// Visit a parse tree produced by PHPParser#magicMethod.
	VisitMagicMethod(ctx *MagicMethodContext) interface{}

	// Visit a parse tree produced by PHPParser#primitiveType.
	VisitPrimitiveType(ctx *PrimitiveTypeContext) interface{}

	// Visit a parse tree produced by PHPParser#castOperation.
	VisitCastOperation(ctx *CastOperationContext) interface{}
}
