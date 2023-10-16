package js2ssa

import (
	"fmt"

	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TAG ssa.ErrorTag = "JS"

// entry point
func (b *astbuilder) build(ast *JS.JavaScriptParser) {
	b.buildStatementList(ast.StatementList().(*JS.StatementListContext))
}

// statement list
func (b *astbuilder) buildStatementList(stmtlist *JS.StatementListContext) {
	recoverRange := b.SetRange(&stmtlist.BaseParserRuleContext)
	defer recoverRange()
	allstmt := stmtlist.AllStatement()
	if len(allstmt) == 0 {
		b.NewError(ssa.Warn, TAG, "empty statement list")
	} else {
		for _, stmt := range allstmt {
			if stmt, ok := stmt.(*JS.StatementContext); ok {
				b.buildStatement(stmt)
			}
		}
	}
}

func (b *astbuilder) buildStatement(stmt *JS.StatementContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.VariableStatement().(*JS.VariableStatementContext); ok {
		b.buildVariableStatement(s)
		return
	}

	if s, ok := stmt.ExpressionStatement().(*JS.ExpressionStatementContext); ok {
		b.buildExpressionStatement(s)
	}

}

func (b *astbuilder) buildVariableStatement(stmt *JS.VariableStatementContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.VariableDeclarationList().(*JS.VariableDeclarationListContext); ok {
		b.buildAllVariableDeclaration(s)
		return
	}

}

func (b *astbuilder) buildAllVariableDeclaration(stmt *JS.VariableDeclarationListContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	for _, jsstmt := range stmt.AllVariableDeclaration() {
		b.buildVariableDeclaration(jsstmt)
		return
	}
}

func (b *astbuilder) buildVariableDeclaration(stmt JS.IVariableDeclarationContext) {
	a := stmt.Assign()
	if a == nil {
		id := stmt.GetText()
		b.WriteVariable(id, ssa.NewAny())
	} else {
		x := stmt.SingleExpression()
		result := b.buildSingleExpression(x, false)
		b.AssignDeclarationExpression(result, stmt)
	}
}

type getSingleExpr interface {
	SingleExpression(i int) JS.ISingleExpressionContext
}

func (b *astbuilder) buildSingleExpression(stmt JS.ISingleExpressionContext, IslValue bool) ssa.Value {
	// TODO: unfinish

	x := stmt
	fmt.Println(x)

	//标识符
	if s, ok := stmt.(*JS.IdentifierExpressionContext); ok {
		ret, result := b.buildIdentifierExpression(s)
		return ret
		fmt.Println(result)
	}



	//字面量
	if s, ok := stmt.(*JS.LiteralExpressionContext); ok {
		return b.buildLiteralExpression(s)
	}

	if s, ok := stmt.(*JS.AssignmentExpressionContext); ok {
		return b.buildAssignmentExpression(s)
	}

	//数学运算
	getValue := func(single getSingleExpr, i int) ssa.Value {
		if s := single.SingleExpression(i); s != nil {
			return b.buildSingleExpression(s, false)
		} else {
			return nil
		}
	}

	getBinaryOp := func() (single getSingleExpr, Op ssa.BinaryOpcode, IsBinOp bool) {
		single, Op, IsBinOp = nil, 0, false
		for {
			a := stmt
			fmt.Println(a.GetText())
			if s := stmt.(*JS.AdditiveExpressionContext); s != nil {
				if op := s.Plus(); op != nil {
					single, Op, IsBinOp = s, ssa.OpAdd, true 
				} else if op := s.Minus(); op != nil {
					single, Op, IsBinOp = s, ssa.OpSub, true
				} else {
					break
				}
			}
			return 
		}
		b.NewError(ssa.Error, TAG, "binary operator not support: %s", stmt.GetText())
		return
	}

	single, opcode, IsBinOp := getBinaryOp()
	if IsBinOp {
		op1 := getValue(single, 0)
		op2 := getValue(single, 1)
		return b.EmitBinOp(opcode, op1, op2)
	}

	return nil
}

func (b *astbuilder) AssignDeclarationExpression(val ssa.Value, stmt JS.IVariableDeclarationContext) {
	// TODO:merge assgin
	b.WriteVariable(stmt.Assignable().GetText(), val)
}

func (b *astbuilder) buildIdentifierExpression(stmt *JS.IdentifierExpressionContext) (ssa.Value, string) {
	identifier := stmt.GetText()
	result := b.ReadVariable(identifier, false)
	if result == nil {
		b.WriteVariable(identifier, ssa.NewAny())
	}
	return b.ReadVariable(identifier, false), identifier
}

func (b *astbuilder) buildAssignmentExpression(stmt *JS.AssignmentExpressionContext) ssa.Value {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	op1 := b.buildSingleExpression(stmt.SingleExpression(0), true)
	op2 := b.buildSingleExpression(stmt.SingleExpression(1), false)
	if op1 != nil && op2 != nil {
		// b.WriteVariable()
	} else {
		b.NewError(ssa.Error, TAG, "AssignmentExpression cannot get right assignable: %s", stmt.GetText())
	}
	return nil
}

func (b *astbuilder) buildExpressionStatement(stmt *JS.ExpressionStatementContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()

	if s, ok := stmt.ExpressionSequence().(*JS.ExpressionSequenceContext); ok {
		b.buildExpressionSequence(s)
	}
}

func (b *astbuilder) buildExpressionSequence(stmt *JS.ExpressionSequenceContext) {
	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
	defer recoverRange()



	for _, expr := range stmt.AllSingleExpression(){
		if s, ok := expr.(JS.ISingleExpressionContext); ok {
			b.buildSingleExpression(s, false)
		}
		return 
	} 
}