// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package parser // YaklangParser

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by YaklangParser.
type YaklangParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by YaklangParser#program.
	VisitProgram(ctx *ProgramContext) interface{}

	// Visit a parse tree produced by YaklangParser#statementList.
	VisitStatementList(ctx *StatementListContext) interface{}

	// Visit a parse tree produced by YaklangParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by YaklangParser#tryStmt.
	VisitTryStmt(ctx *TryStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#expressionStmt.
	VisitExpressionStmt(ctx *ExpressionStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#assignExpressionStmt.
	VisitAssignExpressionStmt(ctx *AssignExpressionStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#lineCommentStmt.
	VisitLineCommentStmt(ctx *LineCommentStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#includeStmt.
	VisitIncludeStmt(ctx *IncludeStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#deferStmt.
	VisitDeferStmt(ctx *DeferStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#goStmt.
	VisitGoStmt(ctx *GoStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#assertStmt.
	VisitAssertStmt(ctx *AssertStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#fallthroughStmt.
	VisitFallthroughStmt(ctx *FallthroughStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#breakStmt.
	VisitBreakStmt(ctx *BreakStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#continueStmt.
	VisitContinueStmt(ctx *ContinueStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#returnStmt.
	VisitReturnStmt(ctx *ReturnStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#forStmt.
	VisitForStmt(ctx *ForStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#forStmtCond.
	VisitForStmtCond(ctx *ForStmtCondContext) interface{}

	// Visit a parse tree produced by YaklangParser#forFirstExpr.
	VisitForFirstExpr(ctx *ForFirstExprContext) interface{}

	// Visit a parse tree produced by YaklangParser#forThirdExpr.
	VisitForThirdExpr(ctx *ForThirdExprContext) interface{}

	// Visit a parse tree produced by YaklangParser#forRangeStmt.
	VisitForRangeStmt(ctx *ForRangeStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#switchStmt.
	VisitSwitchStmt(ctx *SwitchStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#ifStmt.
	VisitIfStmt(ctx *IfStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#elseBlock.
	VisitElseBlock(ctx *ElseBlockContext) interface{}

	// Visit a parse tree produced by YaklangParser#block.
	VisitBlock(ctx *BlockContext) interface{}

	// Visit a parse tree produced by YaklangParser#empty.
	VisitEmpty(ctx *EmptyContext) interface{}

	// Visit a parse tree produced by YaklangParser#inplaceAssignOperator.
	VisitInplaceAssignOperator(ctx *InplaceAssignOperatorContext) interface{}

	// Visit a parse tree produced by YaklangParser#assignExpression.
	VisitAssignExpression(ctx *AssignExpressionContext) interface{}

	// Visit a parse tree produced by YaklangParser#declearVariableExpressionStmt.
	VisitDeclearVariableExpressionStmt(ctx *DeclearVariableExpressionStmtContext) interface{}

	// Visit a parse tree produced by YaklangParser#declearVariableExpression.
	VisitDeclearVariableExpression(ctx *DeclearVariableExpressionContext) interface{}

	// Visit a parse tree produced by YaklangParser#declearVariableOnly.
	VisitDeclearVariableOnly(ctx *DeclearVariableOnlyContext) interface{}

	// Visit a parse tree produced by YaklangParser#declearAndAssignExpression.
	VisitDeclearAndAssignExpression(ctx *DeclearAndAssignExpressionContext) interface{}

	// Visit a parse tree produced by YaklangParser#leftExpressionList.
	VisitLeftExpressionList(ctx *LeftExpressionListContext) interface{}

	// Visit a parse tree produced by YaklangParser#unaryOperator.
	VisitUnaryOperator(ctx *UnaryOperatorContext) interface{}

	// Visit a parse tree produced by YaklangParser#bitBinaryOperator.
	VisitBitBinaryOperator(ctx *BitBinaryOperatorContext) interface{}

	// Visit a parse tree produced by YaklangParser#additiveBinaryOperator.
	VisitAdditiveBinaryOperator(ctx *AdditiveBinaryOperatorContext) interface{}

	// Visit a parse tree produced by YaklangParser#multiplicativeBinaryOperator.
	VisitMultiplicativeBinaryOperator(ctx *MultiplicativeBinaryOperatorContext) interface{}

	// Visit a parse tree produced by YaklangParser#comparisonBinaryOperator.
	VisitComparisonBinaryOperator(ctx *ComparisonBinaryOperatorContext) interface{}

	// Visit a parse tree produced by YaklangParser#leftExpression.
	VisitLeftExpression(ctx *LeftExpressionContext) interface{}

	// Visit a parse tree produced by YaklangParser#leftMemberCall.
	VisitLeftMemberCall(ctx *LeftMemberCallContext) interface{}

	// Visit a parse tree produced by YaklangParser#leftSliceCall.
	VisitLeftSliceCall(ctx *LeftSliceCallContext) interface{}

	// Visit a parse tree produced by YaklangParser#expression.
	VisitExpression(ctx *ExpressionContext) interface{}

	// Visit a parse tree produced by YaklangParser#parenExpression.
	VisitParenExpression(ctx *ParenExpressionContext) interface{}

	// Visit a parse tree produced by YaklangParser#makeExpression.
	VisitMakeExpression(ctx *MakeExpressionContext) interface{}

	// Visit a parse tree produced by YaklangParser#typeLiteral.
	VisitTypeLiteral(ctx *TypeLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#sliceTypeLiteral.
	VisitSliceTypeLiteral(ctx *SliceTypeLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#mapTypeLiteral.
	VisitMapTypeLiteral(ctx *MapTypeLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#instanceCode.
	VisitInstanceCode(ctx *InstanceCodeContext) interface{}

	// Visit a parse tree produced by YaklangParser#anonymousFunctionDecl.
	VisitAnonymousFunctionDecl(ctx *AnonymousFunctionDeclContext) interface{}

	// Visit a parse tree produced by YaklangParser#functionNameDecl.
	VisitFunctionNameDecl(ctx *FunctionNameDeclContext) interface{}

	// Visit a parse tree produced by YaklangParser#functionParamDecl.
	VisitFunctionParamDecl(ctx *FunctionParamDeclContext) interface{}

	// Visit a parse tree produced by YaklangParser#functionCall.
	VisitFunctionCall(ctx *FunctionCallContext) interface{}

	// Visit a parse tree produced by YaklangParser#ordinaryArguments.
	VisitOrdinaryArguments(ctx *OrdinaryArgumentsContext) interface{}

	// Visit a parse tree produced by YaklangParser#memberCall.
	VisitMemberCall(ctx *MemberCallContext) interface{}

	// Visit a parse tree produced by YaklangParser#sliceCall.
	VisitSliceCall(ctx *SliceCallContext) interface{}

	// Visit a parse tree produced by YaklangParser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#numericLiteral.
	VisitNumericLiteral(ctx *NumericLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#stringLiteral.
	VisitStringLiteral(ctx *StringLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#templateSingleQuoteStringLiteral.
	VisitTemplateSingleQuoteStringLiteral(ctx *TemplateSingleQuoteStringLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#templateDoubleQuoteStringLiteral.
	VisitTemplateDoubleQuoteStringLiteral(ctx *TemplateDoubleQuoteStringLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#templateBackTickStringLiteral.
	VisitTemplateBackTickStringLiteral(ctx *TemplateBackTickStringLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#templateStringLiteral.
	VisitTemplateStringLiteral(ctx *TemplateStringLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#templateSingleQupteStringAtom.
	VisitTemplateSingleQupteStringAtom(ctx *TemplateSingleQupteStringAtomContext) interface{}

	// Visit a parse tree produced by YaklangParser#templateDoubleQupteStringAtom.
	VisitTemplateDoubleQupteStringAtom(ctx *TemplateDoubleQupteStringAtomContext) interface{}

	// Visit a parse tree produced by YaklangParser#templateBackTickStringAtom.
	VisitTemplateBackTickStringAtom(ctx *TemplateBackTickStringAtomContext) interface{}

	// Visit a parse tree produced by YaklangParser#boolLiteral.
	VisitBoolLiteral(ctx *BoolLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#charaterLiteral.
	VisitCharaterLiteral(ctx *CharaterLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#sliceLiteral.
	VisitSliceLiteral(ctx *SliceLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#sliceTypedLiteral.
	VisitSliceTypedLiteral(ctx *SliceTypedLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#expressionList.
	VisitExpressionList(ctx *ExpressionListContext) interface{}

	// Visit a parse tree produced by YaklangParser#expressionListMultiline.
	VisitExpressionListMultiline(ctx *ExpressionListMultilineContext) interface{}

	// Visit a parse tree produced by YaklangParser#mapLiteral.
	VisitMapLiteral(ctx *MapLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#mapTypedLiteral.
	VisitMapTypedLiteral(ctx *MapTypedLiteralContext) interface{}

	// Visit a parse tree produced by YaklangParser#mapPairs.
	VisitMapPairs(ctx *MapPairsContext) interface{}

	// Visit a parse tree produced by YaklangParser#mapPair.
	VisitMapPair(ctx *MapPairContext) interface{}

	// Visit a parse tree produced by YaklangParser#ws.
	VisitWs(ctx *WsContext) interface{}

	// Visit a parse tree produced by YaklangParser#eos.
	VisitEos(ctx *EosContext) interface{}
}
