// Code generated from ./JavaScriptParser.g4 by ANTLR 4.13.0. DO NOT EDIT.

package parser // JavaScriptParser

import "github.com/antlr4-go/antlr/v4"

// A complete Visitor for a parse tree produced by JavaScriptParser.
type JavaScriptParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by JavaScriptParser#program.
	VisitProgram(ctx *ProgramContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#sourceElement.
	VisitSourceElement(ctx *SourceElementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#block.
	VisitBlock(ctx *BlockContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#statementList.
	VisitStatementList(ctx *StatementListContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#importStatement.
	VisitImportStatement(ctx *ImportStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#importFromBlock.
	VisitImportFromBlock(ctx *ImportFromBlockContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#importModuleItems.
	VisitImportModuleItems(ctx *ImportModuleItemsContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#importAliasName.
	VisitImportAliasName(ctx *ImportAliasNameContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#moduleExportName.
	VisitModuleExportName(ctx *ModuleExportNameContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#importedBinding.
	VisitImportedBinding(ctx *ImportedBindingContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#importDefault.
	VisitImportDefault(ctx *ImportDefaultContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#importNamespace.
	VisitImportNamespace(ctx *ImportNamespaceContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#importFrom.
	VisitImportFrom(ctx *ImportFromContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#aliasName.
	VisitAliasName(ctx *AliasNameContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ExportDeclaration.
	VisitExportDeclaration(ctx *ExportDeclarationContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ExportDefaultDeclaration.
	VisitExportDefaultDeclaration(ctx *ExportDefaultDeclarationContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#exportFromBlock.
	VisitExportFromBlock(ctx *ExportFromBlockContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#exportModuleItems.
	VisitExportModuleItems(ctx *ExportModuleItemsContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#exportAliasName.
	VisitExportAliasName(ctx *ExportAliasNameContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#declaration.
	VisitDeclaration(ctx *DeclarationContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#variableStatement.
	VisitVariableStatement(ctx *VariableStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#variableDeclarationList.
	VisitVariableDeclarationList(ctx *VariableDeclarationListContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#variableDeclaration.
	VisitVariableDeclaration(ctx *VariableDeclarationContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#emptyStatement_.
	VisitEmptyStatement_(ctx *EmptyStatement_Context) interface{}

	// Visit a parse tree produced by JavaScriptParser#expressionStatement.
	VisitExpressionStatement(ctx *ExpressionStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ifStatement.
	VisitIfStatement(ctx *IfStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#DoStatement.
	VisitDoStatement(ctx *DoStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#WhileStatement.
	VisitWhileStatement(ctx *WhileStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ForStatement.
	VisitForStatement(ctx *ForStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ForInStatement.
	VisitForInStatement(ctx *ForInStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ForOfStatement.
	VisitForOfStatement(ctx *ForOfStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#varModifier.
	VisitVarModifier(ctx *VarModifierContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#continueStatement.
	VisitContinueStatement(ctx *ContinueStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#breakStatement.
	VisitBreakStatement(ctx *BreakStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#returnStatement.
	VisitReturnStatement(ctx *ReturnStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#yieldStatement.
	VisitYieldStatement(ctx *YieldStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#withStatement.
	VisitWithStatement(ctx *WithStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#switchStatement.
	VisitSwitchStatement(ctx *SwitchStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#caseBlock.
	VisitCaseBlock(ctx *CaseBlockContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#caseClauses.
	VisitCaseClauses(ctx *CaseClausesContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#caseClause.
	VisitCaseClause(ctx *CaseClauseContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#defaultClause.
	VisitDefaultClause(ctx *DefaultClauseContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#labelledStatement.
	VisitLabelledStatement(ctx *LabelledStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#throwStatement.
	VisitThrowStatement(ctx *ThrowStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#tryStatement.
	VisitTryStatement(ctx *TryStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#catchProduction.
	VisitCatchProduction(ctx *CatchProductionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#finallyProduction.
	VisitFinallyProduction(ctx *FinallyProductionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#debuggerStatement.
	VisitDebuggerStatement(ctx *DebuggerStatementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#functionDeclaration.
	VisitFunctionDeclaration(ctx *FunctionDeclarationContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#classDeclaration.
	VisitClassDeclaration(ctx *ClassDeclarationContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#classTail.
	VisitClassTail(ctx *ClassTailContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#classElement.
	VisitClassElement(ctx *ClassElementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#methodDefinition.
	VisitMethodDefinition(ctx *MethodDefinitionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#fieldDefinition.
	VisitFieldDefinition(ctx *FieldDefinitionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#classElementName.
	VisitClassElementName(ctx *ClassElementNameContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#privateIdentifier.
	VisitPrivateIdentifier(ctx *PrivateIdentifierContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#formalParameterList.
	VisitFormalParameterList(ctx *FormalParameterListContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#formalParameterArg.
	VisitFormalParameterArg(ctx *FormalParameterArgContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#lastFormalParameterArg.
	VisitLastFormalParameterArg(ctx *LastFormalParameterArgContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#functionBody.
	VisitFunctionBody(ctx *FunctionBodyContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#sourceElements.
	VisitSourceElements(ctx *SourceElementsContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#arrayLiteral.
	VisitArrayLiteral(ctx *ArrayLiteralContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#elementList.
	VisitElementList(ctx *ElementListContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#arrayElement.
	VisitArrayElement(ctx *ArrayElementContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#PropertyExpressionAssignment.
	VisitPropertyExpressionAssignment(ctx *PropertyExpressionAssignmentContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ComputedPropertyExpressionAssignment.
	VisitComputedPropertyExpressionAssignment(ctx *ComputedPropertyExpressionAssignmentContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#FunctionProperty.
	VisitFunctionProperty(ctx *FunctionPropertyContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#PropertyGetter.
	VisitPropertyGetter(ctx *PropertyGetterContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#PropertySetter.
	VisitPropertySetter(ctx *PropertySetterContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#PropertyShorthand.
	VisitPropertyShorthand(ctx *PropertyShorthandContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#propertyName.
	VisitPropertyName(ctx *PropertyNameContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#arguments.
	VisitArguments(ctx *ArgumentsContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#argument.
	VisitArgument(ctx *ArgumentContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#expressionSequence.
	VisitExpressionSequence(ctx *ExpressionSequenceContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#TemplateStringExpression.
	VisitTemplateStringExpression(ctx *TemplateStringExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#TernaryExpression.
	VisitTernaryExpression(ctx *TernaryExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#LogicalAndExpression.
	VisitLogicalAndExpression(ctx *LogicalAndExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#PowerExpression.
	VisitPowerExpression(ctx *PowerExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#PreIncrementExpression.
	VisitPreIncrementExpression(ctx *PreIncrementExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ObjectLiteralExpression.
	VisitObjectLiteralExpression(ctx *ObjectLiteralExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#MetaExpression.
	VisitMetaExpression(ctx *MetaExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#InExpression.
	VisitInExpression(ctx *InExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#LogicalOrExpression.
	VisitLogicalOrExpression(ctx *LogicalOrExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#OptionalChainExpression.
	VisitOptionalChainExpression(ctx *OptionalChainExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#NotExpression.
	VisitNotExpression(ctx *NotExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#PreDecreaseExpression.
	VisitPreDecreaseExpression(ctx *PreDecreaseExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ArgumentsExpression.
	VisitArgumentsExpression(ctx *ArgumentsExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#AwaitExpression.
	VisitAwaitExpression(ctx *AwaitExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ThisExpression.
	VisitThisExpression(ctx *ThisExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#FunctionExpression.
	VisitFunctionExpression(ctx *FunctionExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#UnaryMinusExpression.
	VisitUnaryMinusExpression(ctx *UnaryMinusExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#AssignmentExpression.
	VisitAssignmentExpression(ctx *AssignmentExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#PostDecreaseExpression.
	VisitPostDecreaseExpression(ctx *PostDecreaseExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#TypeofExpression.
	VisitTypeofExpression(ctx *TypeofExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#InstanceofExpression.
	VisitInstanceofExpression(ctx *InstanceofExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#UnaryPlusExpression.
	VisitUnaryPlusExpression(ctx *UnaryPlusExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#DeleteExpression.
	VisitDeleteExpression(ctx *DeleteExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ImportExpression.
	VisitImportExpression(ctx *ImportExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#EqualityExpression.
	VisitEqualityExpression(ctx *EqualityExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#BitXOrExpression.
	VisitBitXOrExpression(ctx *BitXOrExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#SuperExpression.
	VisitSuperExpression(ctx *SuperExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#MultiplicativeExpression.
	VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#BitShiftExpression.
	VisitBitShiftExpression(ctx *BitShiftExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ParenthesizedExpression.
	VisitParenthesizedExpression(ctx *ParenthesizedExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#AdditiveExpression.
	VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#RelationalExpression.
	VisitRelationalExpression(ctx *RelationalExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#PostIncrementExpression.
	VisitPostIncrementExpression(ctx *PostIncrementExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#YieldExpression.
	VisitYieldExpression(ctx *YieldExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#BitNotExpression.
	VisitBitNotExpression(ctx *BitNotExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#NewExpression.
	VisitNewExpression(ctx *NewExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#LiteralExpression.
	VisitLiteralExpression(ctx *LiteralExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ArrayLiteralExpression.
	VisitArrayLiteralExpression(ctx *ArrayLiteralExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#MemberDotExpression.
	VisitMemberDotExpression(ctx *MemberDotExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ClassExpression.
	VisitClassExpression(ctx *ClassExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#MemberIndexExpression.
	VisitMemberIndexExpression(ctx *MemberIndexExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#IdentifierExpression.
	VisitIdentifierExpression(ctx *IdentifierExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#BitAndExpression.
	VisitBitAndExpression(ctx *BitAndExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#BitOrExpression.
	VisitBitOrExpression(ctx *BitOrExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#AssignmentOperatorExpression.
	VisitAssignmentOperatorExpression(ctx *AssignmentOperatorExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#VoidExpression.
	VisitVoidExpression(ctx *VoidExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#CoalesceExpression.
	VisitCoalesceExpression(ctx *CoalesceExpressionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#initializer.
	VisitInitializer(ctx *InitializerContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#assignable.
	VisitAssignable(ctx *AssignableContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#objectLiteral.
	VisitObjectLiteral(ctx *ObjectLiteralContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#AnonymousFunctionDecl.
	VisitAnonymousFunctionDecl(ctx *AnonymousFunctionDeclContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#ArrowFunction.
	VisitArrowFunction(ctx *ArrowFunctionContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#arrowFunctionParameters.
	VisitArrowFunctionParameters(ctx *ArrowFunctionParametersContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#arrowFunctionBody.
	VisitArrowFunctionBody(ctx *ArrowFunctionBodyContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#assignmentOperator.
	VisitAssignmentOperator(ctx *AssignmentOperatorContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#templateStringLiteral.
	VisitTemplateStringLiteral(ctx *TemplateStringLiteralContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#templateStringAtom.
	VisitTemplateStringAtom(ctx *TemplateStringAtomContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#numericLiteral.
	VisitNumericLiteral(ctx *NumericLiteralContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#bigintLiteral.
	VisitBigintLiteral(ctx *BigintLiteralContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#getter.
	VisitGetter(ctx *GetterContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#setter.
	VisitSetter(ctx *SetterContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#identifierName.
	VisitIdentifierName(ctx *IdentifierNameContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#identifier.
	VisitIdentifier(ctx *IdentifierContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#reservedWord.
	VisitReservedWord(ctx *ReservedWordContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#keyword.
	VisitKeyword(ctx *KeywordContext) interface{}

	// Visit a parse tree produced by JavaScriptParser#let_.
	VisitLet_(ctx *Let_Context) interface{}

	// Visit a parse tree produced by JavaScriptParser#eos.
	VisitEos(ctx *EosContext) interface{}
}
