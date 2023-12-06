// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package phpparser // PHPParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type BasePHPParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BasePHPParserVisitor) VisitHtmlDocument(ctx *HtmlDocumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitHtmlDocumentElement(ctx *HtmlDocumentElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitInlineHtml(ctx *InlineHtmlContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitHtmlElement(ctx *HtmlElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitScriptText(ctx *ScriptTextContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitPhpBlock(ctx *PhpBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitImportStatement(ctx *ImportStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTopStatement(ctx *TopStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitUseDeclaration(ctx *UseDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitUseDeclarationContentList(ctx *UseDeclarationContentListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitUseDeclarationContent(ctx *UseDeclarationContentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitNamespaceDeclaration(ctx *NamespaceDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitNamespaceStatement(ctx *NamespaceStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitFunctionDeclaration(ctx *FunctionDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitClassDeclaration(ctx *ClassDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitClassEntryType(ctx *ClassEntryTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitInterfaceList(ctx *InterfaceListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTypeParameterListInBrackets(ctx *TypeParameterListInBracketsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTypeParameterList(ctx *TypeParameterListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTypeParameterWithDefaultsList(ctx *TypeParameterWithDefaultsListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTypeParameterDecl(ctx *TypeParameterDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTypeParameterWithDefaultDecl(ctx *TypeParameterWithDefaultDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitGenericDynamicArgs(ctx *GenericDynamicArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitAttributes(ctx *AttributesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitAttributeGroup(ctx *AttributeGroupContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitAttribute(ctx *AttributeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitInnerStatementList(ctx *InnerStatementListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitInnerStatement(ctx *InnerStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitLabelStatement(ctx *LabelStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitEmptyStatement_(ctx *EmptyStatement_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitBlockStatement(ctx *BlockStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitIfStatement(ctx *IfStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitElseIfStatement(ctx *ElseIfStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitElseIfColonStatement(ctx *ElseIfColonStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitElseStatement(ctx *ElseStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitElseColonStatement(ctx *ElseColonStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitWhileStatement(ctx *WhileStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitDoWhileStatement(ctx *DoWhileStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitForStatement(ctx *ForStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitForInit(ctx *ForInitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitForUpdate(ctx *ForUpdateContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitSwitchStatement(ctx *SwitchStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitSwitchBlock(ctx *SwitchBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitBreakStatement(ctx *BreakStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitContinueStatement(ctx *ContinueStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitReturnStatement(ctx *ReturnStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitExpressionStatement(ctx *ExpressionStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitUnsetStatement(ctx *UnsetStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitForeachStatement(ctx *ForeachStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTryCatchFinally(ctx *TryCatchFinallyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitCatchClause(ctx *CatchClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitFinallyStatement(ctx *FinallyStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitThrowStatement(ctx *ThrowStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitGotoStatement(ctx *GotoStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitDeclareStatement(ctx *DeclareStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitInlineHtmlStatement(ctx *InlineHtmlStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitDeclareList(ctx *DeclareListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitDirective(ctx *DirectiveContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitFormalParameterList(ctx *FormalParameterListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitFormalParameter(ctx *FormalParameterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTypeHint(ctx *TypeHintContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitGlobalStatement(ctx *GlobalStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitGlobalVar(ctx *GlobalVarContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitEchoStatement(ctx *EchoStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitStaticVariableStatement(ctx *StaticVariableStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitClassStatement(ctx *ClassStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTraitAdaptations(ctx *TraitAdaptationsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTraitAdaptationStatement(ctx *TraitAdaptationStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTraitPrecedence(ctx *TraitPrecedenceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTraitAlias(ctx *TraitAliasContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTraitMethodReference(ctx *TraitMethodReferenceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitBaseCtorCall(ctx *BaseCtorCallContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitReturnTypeDecl(ctx *ReturnTypeDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitMethodBody(ctx *MethodBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitPropertyModifiers(ctx *PropertyModifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitMemberModifiers(ctx *MemberModifiersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitVariableInitializer(ctx *VariableInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitIdentifierInitializer(ctx *IdentifierInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitGlobalConstantDeclaration(ctx *GlobalConstantDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitEnumDeclaration(ctx *EnumDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitEnumItem(ctx *EnumItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitExpressionList(ctx *ExpressionListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitParentheses(ctx *ParenthesesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitChainExpression(ctx *ChainExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitSpecialWordExpression(ctx *SpecialWordExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitArrayCreationExpression(ctx *ArrayCreationExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitBackQuoteStringExpression(ctx *BackQuoteStringExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitKeywordNewExpression(ctx *KeywordNewExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitMatchExpression(ctx *MatchExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitLogicalExpression(ctx *LogicalExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitPrintExpression(ctx *PrintExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitAssignmentExpression(ctx *AssignmentExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitPostfixIncDecExpression(ctx *PostfixIncDecExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitCloneExpression(ctx *CloneExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitUnaryOperatorExpression(ctx *UnaryOperatorExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitParenthesisExpression(ctx *ParenthesisExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitSpaceshipExpression(ctx *SpaceshipExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitConditionalExpression(ctx *ConditionalExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitNullCoalescingExpression(ctx *NullCoalescingExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitArithmeticExpression(ctx *ArithmeticExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitIndexerExpression(ctx *IndexerExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitScalarExpression(ctx *ScalarExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitPrefixIncDecExpression(ctx *PrefixIncDecExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitComparisonExpression(ctx *ComparisonExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitCastExpression(ctx *CastExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitInstanceOfExpression(ctx *InstanceOfExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitArrayDestructExpression(ctx *ArrayDestructExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitLambdaFunctionExpression(ctx *LambdaFunctionExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitBitwiseExpression(ctx *BitwiseExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitAssignable(ctx *AssignableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitArrayCreation(ctx *ArrayCreationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitArrayDestructuring(ctx *ArrayDestructuringContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitIndexedDestructItem(ctx *IndexedDestructItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitKeyedDestructItem(ctx *KeyedDestructItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitLambdaFunctionExpr(ctx *LambdaFunctionExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitMatchExpr(ctx *MatchExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitMatchItem(ctx *MatchItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitNewExpr(ctx *NewExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitAssignmentOperator(ctx *AssignmentOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitYieldExpression(ctx *YieldExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitArrayItemList(ctx *ArrayItemListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitArrayItem(ctx *ArrayItemContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitLambdaFunctionUseVars(ctx *LambdaFunctionUseVarsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitLambdaFunctionUseVar(ctx *LambdaFunctionUseVarContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitQualifiedStaticTypeRef(ctx *QualifiedStaticTypeRefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitTypeRef(ctx *TypeRefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitAnonymousClass(ctx *AnonymousClassContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitIndirectTypeRef(ctx *IndirectTypeRefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitQualifiedNamespaceName(ctx *QualifiedNamespaceNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitNamespaceNameList(ctx *NamespaceNameListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitNamespaceNameTail(ctx *NamespaceNameTailContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitQualifiedNamespaceNameList(ctx *QualifiedNamespaceNameListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitArguments(ctx *ArgumentsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitActualArgument(ctx *ActualArgumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitArgumentName(ctx *ArgumentNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitConstantInitializer(ctx *ConstantInitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitConstant(ctx *ConstantContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitLiteralConstant(ctx *LiteralConstantContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitNumericConstant(ctx *NumericConstantContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitClassConstant(ctx *ClassConstantContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitStringConstant(ctx *StringConstantContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitString(ctx *StringContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitInterpolatedStringPart(ctx *InterpolatedStringPartContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitChainList(ctx *ChainListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitChain(ctx *ChainContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitChainOrigin(ctx *ChainOriginContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitMemberAccess(ctx *MemberAccessContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitFunctionCall(ctx *FunctionCallContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitFunctionCallName(ctx *FunctionCallNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitActualArguments(ctx *ActualArgumentsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitChainBase(ctx *ChainBaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitKeyedFieldName(ctx *KeyedFieldNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitKeyedSimpleFieldName(ctx *KeyedSimpleFieldNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitKeyedVariable(ctx *KeyedVariableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitSquareCurlyExpression(ctx *SquareCurlyExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitAssignmentList(ctx *AssignmentListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitAssignmentListElement(ctx *AssignmentListElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitModifier(ctx *ModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitIdentifier(ctx *IdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitMemberModifier(ctx *MemberModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitMagicConstant(ctx *MagicConstantContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitMagicMethod(ctx *MagicMethodContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitPrimitiveType(ctx *PrimitiveTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BasePHPParserVisitor) VisitCastOperation(ctx *CastOperationContext) interface{} {
	return v.VisitChildren(ctx)
}
