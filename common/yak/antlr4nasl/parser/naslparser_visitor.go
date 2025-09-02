// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package parser // NaslParser

import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by NaslParser.
type NaslParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by NaslParser#program.
	VisitProgram(ctx *ProgramContext) interface{}

	// Visit a parse tree produced by NaslParser#statementList.
	VisitStatementList(ctx *StatementListContext) interface{}

	// Visit a parse tree produced by NaslParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by NaslParser#block.
	VisitBlock(ctx *BlockContext) interface{}

	// Visit a parse tree produced by NaslParser#variableDeclarationStatement.
	VisitVariableDeclarationStatement(ctx *VariableDeclarationStatementContext) interface{}

	// Visit a parse tree produced by NaslParser#variableAssignStatement.
	VisitVariableAssignStatement(ctx *VariableAssignStatementContext) interface{}

	// Visit a parse tree produced by NaslParser#expressionStatement.
	VisitExpressionStatement(ctx *ExpressionStatementContext) interface{}

	// Visit a parse tree produced by NaslParser#ifStatement.
	VisitIfStatement(ctx *IfStatementContext) interface{}

	// Visit a parse tree produced by NaslParser#TraditionalFor.
	VisitTraditionalFor(ctx *TraditionalForContext) interface{}

	// Visit a parse tree produced by NaslParser#ForEach.
	VisitForEach(ctx *ForEachContext) interface{}

	// Visit a parse tree produced by NaslParser#While.
	VisitWhile(ctx *WhileContext) interface{}

	// Visit a parse tree produced by NaslParser#Repeat.
	VisitRepeat(ctx *RepeatContext) interface{}

	// Visit a parse tree produced by NaslParser#continueStatement.
	VisitContinueStatement(ctx *ContinueStatementContext) interface{}

	// Visit a parse tree produced by NaslParser#breakStatement.
	VisitBreakStatement(ctx *BreakStatementContext) interface{}

	// Visit a parse tree produced by NaslParser#returnStatement.
	VisitReturnStatement(ctx *ReturnStatementContext) interface{}

	// Visit a parse tree produced by NaslParser#argumentList.
	VisitArgumentList(ctx *ArgumentListContext) interface{}

	// Visit a parse tree produced by NaslParser#argument.
	VisitArgument(ctx *ArgumentContext) interface{}

	// Visit a parse tree produced by NaslParser#expressionSequence.
	VisitExpressionSequence(ctx *ExpressionSequenceContext) interface{}

	// Visit a parse tree produced by NaslParser#functionDeclarationStatement.
	VisitFunctionDeclarationStatement(ctx *FunctionDeclarationStatementContext) interface{}

	// Visit a parse tree produced by NaslParser#parameterList.
	VisitParameterList(ctx *ParameterListContext) interface{}

	// Visit a parse tree produced by NaslParser#arrayLiteral.
	VisitArrayLiteral(ctx *ArrayLiteralContext) interface{}

	// Visit a parse tree produced by NaslParser#elementList.
	VisitElementList(ctx *ElementListContext) interface{}

	// Visit a parse tree produced by NaslParser#arrayElement.
	VisitArrayElement(ctx *ArrayElementContext) interface{}

	// Visit a parse tree produced by NaslParser#LogicalAndExpression.
	VisitLogicalAndExpression(ctx *LogicalAndExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#PreIncrementExpression.
	VisitPreIncrementExpression(ctx *PreIncrementExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#LogicalOrExpression.
	VisitLogicalOrExpression(ctx *LogicalOrExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#NotExpression.
	VisitNotExpression(ctx *NotExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#PreDecreaseExpression.
	VisitPreDecreaseExpression(ctx *PreDecreaseExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#UnaryMinusExpression.
	VisitUnaryMinusExpression(ctx *UnaryMinusExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#AssignmentExpression.
	VisitAssignmentExpression(ctx *AssignmentExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#PostDecreaseExpression.
	VisitPostDecreaseExpression(ctx *PostDecreaseExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#UnaryPlusExpression.
	VisitUnaryPlusExpression(ctx *UnaryPlusExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#EqualityExpression.
	VisitEqualityExpression(ctx *EqualityExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#BitXOrExpression.
	VisitBitXOrExpression(ctx *BitXOrExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#MultiplicativeExpression.
	VisitMultiplicativeExpression(ctx *MultiplicativeExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#CallExpression.
	VisitCallExpression(ctx *CallExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#BitShiftExpression.
	VisitBitShiftExpression(ctx *BitShiftExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#ParenthesizedExpression.
	VisitParenthesizedExpression(ctx *ParenthesizedExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#AdditiveExpression.
	VisitAdditiveExpression(ctx *AdditiveExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#RelationalExpression.
	VisitRelationalExpression(ctx *RelationalExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#PostIncrementExpression.
	VisitPostIncrementExpression(ctx *PostIncrementExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#BitNotExpression.
	VisitBitNotExpression(ctx *BitNotExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#LiteralExpression.
	VisitLiteralExpression(ctx *LiteralExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#ArrayLiteralExpression.
	VisitArrayLiteralExpression(ctx *ArrayLiteralExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#MemberDotExpression.
	VisitMemberDotExpression(ctx *MemberDotExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#MemberIndexExpression.
	VisitMemberIndexExpression(ctx *MemberIndexExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#IdentifierExpression.
	VisitIdentifierExpression(ctx *IdentifierExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#BitAndExpression.
	VisitBitAndExpression(ctx *BitAndExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#BitOrExpression.
	VisitBitOrExpression(ctx *BitOrExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#XExpression.
	VisitXExpression(ctx *XExpressionContext) interface{}

	// Visit a parse tree produced by NaslParser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}

	// Visit a parse tree produced by NaslParser#numericLiteral.
	VisitNumericLiteral(ctx *NumericLiteralContext) interface{}

	// Visit a parse tree produced by NaslParser#identifier.
	VisitIdentifier(ctx *IdentifierContext) interface{}

	// Visit a parse tree produced by NaslParser#assignmentOperator.
	VisitAssignmentOperator(ctx *AssignmentOperatorContext) interface{}

	// Visit a parse tree produced by NaslParser#eos.
	VisitEos(ctx *EosContext) interface{}
}
