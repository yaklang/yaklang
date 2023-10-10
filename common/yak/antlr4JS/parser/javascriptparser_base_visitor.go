// Code generated from ./JavaScriptParser.g4 by ANTLR 4.13.0. DO NOT EDIT.

package parser // JavaScriptParser

import "github.com/antlr4-go/antlr/v4"

type BaseJavaScriptParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseJavaScriptParserVisitor) VisitProgram(ctx *ProgramContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitSourceElement(ctx *SourceElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitBlock(ctx *BlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitStatementList(ctx *StatementListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitImportStatement(ctx *ImportStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitImportFromBlock(ctx *ImportFromBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitImportModuleItems(ctx *ImportModuleItemsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitImportAliasName(ctx *ImportAliasNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitModuleExportName(ctx *ModuleExportNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitImportedBinding(ctx *ImportedBindingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitImportDefault(ctx *ImportDefaultContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitImportNamespace(ctx *ImportNamespaceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitImportFrom(ctx *ImportFromContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitAliasName(ctx *AliasNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitExportDeclaration(ctx *ExportDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitExportDefaultDeclaration(ctx *ExportDefaultDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitExportFromBlock(ctx *ExportFromBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitExportModuleItems(ctx *ExportModuleItemsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitExportAliasName(ctx *ExportAliasNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitDeclaration(ctx *DeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitVariableStatement(ctx *VariableStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitVariableDeclarationList(ctx *VariableDeclarationListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitVariableDeclaration(ctx *VariableDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitEmptyStatement_(ctx *EmptyStatement_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitExpressionStatement(ctx *ExpressionStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitIfStatement(ctx *IfStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitDoStatement(ctx *DoStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitWhileStatement(ctx *WhileStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitForStatement(ctx *ForStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitForInStatement(ctx *ForInStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitForOfStatement(ctx *ForOfStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitVarModifier(ctx *VarModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitContinueStatement(ctx *ContinueStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitBreakStatement(ctx *BreakStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitReturnStatement(ctx *ReturnStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitYieldStatement(ctx *YieldStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitWithStatement(ctx *WithStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitSwitchStatement(ctx *SwitchStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitCaseBlock(ctx *CaseBlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitCaseClauses(ctx *CaseClausesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitCaseClause(ctx *CaseClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitDefaultClause(ctx *DefaultClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitLabelledStatement(ctx *LabelledStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitThrowStatement(ctx *ThrowStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitTryStatement(ctx *TryStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitCatchProduction(ctx *CatchProductionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitFinallyProduction(ctx *FinallyProductionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitDebuggerStatement(ctx *DebuggerStatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitFunctionDeclaration(ctx *FunctionDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitClassDeclaration(ctx *ClassDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitClassTail(ctx *ClassTailContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitClassElement(ctx *ClassElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitMethodDefinition(ctx *MethodDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitFieldDefinition(ctx *FieldDefinitionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitClassElementName(ctx *ClassElementNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPrivateIdentifier(ctx *PrivateIdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitFormalParameterList(ctx *FormalParameterListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitFormalParameterArg(ctx *FormalParameterArgContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitLastFormalParameterArg(ctx *LastFormalParameterArgContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitFunctionBody(ctx *FunctionBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitSourceElements(ctx *SourceElementsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitArrayLiteral(ctx *ArrayLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitElementList(ctx *ElementListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitArrayElement(ctx *ArrayElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPropertyExpressionAssignment(ctx *PropertyExpressionAssignmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitComputedPropertyExpressionAssignment(ctx *ComputedPropertyExpressionAssignmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitFunctionProperty(ctx *FunctionPropertyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPropertyGetter(ctx *PropertyGetterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPropertySetter(ctx *PropertySetterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPropertyShorthand(ctx *PropertyShorthandContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPropertyName(ctx *PropertyNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitArguments(ctx *ArgumentsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitArgument(ctx *ArgumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitExpressionSequence(ctx *ExpressionSequenceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitTemplateStringExpression(ctx *TemplateStringExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitTernaryExpression(ctx *TernaryExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitLogicalAndExpression(ctx *LogicalAndExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPowerExpression(ctx *PowerExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPreIncrementExpression(ctx *PreIncrementExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitObjectLiteralExpression(ctx *ObjectLiteralExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitMetaExpression(ctx *MetaExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitInExpression(ctx *InExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitLogicalOrExpression(ctx *LogicalOrExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitOptionalChainExpression(ctx *OptionalChainExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitNotExpression(ctx *NotExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPreDecreaseExpression(ctx *PreDecreaseExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitArgumentsExpression(ctx *ArgumentsExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitAwaitExpression(ctx *AwaitExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitThisExpression(ctx *ThisExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitFunctionExpression(ctx *FunctionExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitUnaryMinusExpression(ctx *UnaryMinusExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitAssignmentExpression(ctx *AssignmentExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPostDecreaseExpression(ctx *PostDecreaseExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitTypeofExpression(ctx *TypeofExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitInstanceofExpression(ctx *InstanceofExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitUnaryPlusExpression(ctx *UnaryPlusExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitDeleteExpression(ctx *DeleteExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitImportExpression(ctx *ImportExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitEqualityExpression(ctx *EqualityExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitBitXOrExpression(ctx *BitXOrExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitSuperExpression(ctx *SuperExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitBitShiftExpression(ctx *BitShiftExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitParenthesizedExpression(ctx *ParenthesizedExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitRelationalExpression(ctx *RelationalExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitPostIncrementExpression(ctx *PostIncrementExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitYieldExpression(ctx *YieldExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitBitNotExpression(ctx *BitNotExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitNewExpression(ctx *NewExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitLiteralExpression(ctx *LiteralExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitArrayLiteralExpression(ctx *ArrayLiteralExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitMemberDotExpression(ctx *MemberDotExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitClassExpression(ctx *ClassExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitMemberIndexExpression(ctx *MemberIndexExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitIdentifierExpression(ctx *IdentifierExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitBitAndExpression(ctx *BitAndExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitBitOrExpression(ctx *BitOrExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitAssignmentOperatorExpression(ctx *AssignmentOperatorExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitVoidExpression(ctx *VoidExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitCoalesceExpression(ctx *CoalesceExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitInitializer(ctx *InitializerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitAssignable(ctx *AssignableContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitObjectLiteral(ctx *ObjectLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitAnonymousFunctionDecl(ctx *AnonymousFunctionDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitArrowFunction(ctx *ArrowFunctionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitArrowFunctionParameters(ctx *ArrowFunctionParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitArrowFunctionBody(ctx *ArrowFunctionBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitAssignmentOperator(ctx *AssignmentOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitLiteral(ctx *LiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitTemplateStringLiteral(ctx *TemplateStringLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitTemplateStringAtom(ctx *TemplateStringAtomContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitNumericLiteral(ctx *NumericLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitBigintLiteral(ctx *BigintLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitGetter(ctx *GetterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitSetter(ctx *SetterContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitIdentifierName(ctx *IdentifierNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitIdentifier(ctx *IdentifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitReservedWord(ctx *ReservedWordContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitKeyword(ctx *KeywordContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitLet_(ctx *Let_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseJavaScriptParserVisitor) VisitEos(ctx *EosContext) interface{} {
	return v.VisitChildren(ctx)
}
