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
		result := b.buildSingleExpression(x)
		b.AssignExpression(result, stmt)
	}
}

type getSingleExpr interface {
	SingleExpression(i int) JS.ISingleExpressionContext
}

func (b *astbuilder) buildSingleExpression(stmt JS.ISingleExpressionContext) ssa.Value {
	// TODO: unfinish

	var result ssa.Value
	x := stmt
	fmt.Println(x)

	//字面量
	if s, ok := stmt.(*JS.LiteralExpressionContext); ok {
		return b.buildLiteralExpression(s)
	}

	//数学运算
	getValue := func(single getSingleExpr, i int) ssa.Value {
		if s := single.SingleExpression(i); s != nil {
			return b.buildSingleExpression(s)
		} else {
			return nil
		}
	}

	getBinaryOp := func() (single getSingleExpr, Op ssa.BinaryOpcode, IsBinOp bool) {
		single, Op, IsBinOp = nil, 0, false
		for {
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

	// //数学运算
	// if s, ok := stmt.(*JS.AdditiveExpressionContext); ok {
	// 	op1 := getValue(s, 0)
	// 	op2 := getValue(s, 1)

	// 	if op := s.Plus(); op != nil {
	// 		return b.EmitBinOp(ssa.OpAdd, op1, op2)
	// 	} else if op := s.Minus(); op != nil {
	// 		return b.EmitBinOp(ssa.OpSub, op1, op2)
	// 	} else {
	// 		b.NewError(ssa.Error, TAG, "binary operator not support: %s", op.GetText())
	// 		return nil
	// 	}
	// }

	// if s := stmt.(*JS.MultiplicativeExpressionContext); s != nil {
	// 	op1 := getValue(s, 0)
	// 	op2 := getValue(s, 1)

	// 	if op := s.Multiply(); op != nil {
	// 		return b.EmitBinOp(ssa.OpMul, op1, op2)
	// 	} else if op := s.Divide(); op != nil {
	// 		return b.EmitBinOp(ssa.OpDiv, op1, op2)
	// 	} else if op := s.Modulus(); op != nil {
	// 		return b.EmitBinOp(ssa.OpMod, op1, op2)
	// 	} else {
	// 		b.NewError(ssa.Error, TAG, "binary operator not support: %s", op.GetText())
	// 		return nil
	// 	}
	// }

	// if s := stmt.(*JS.RelationalExpressionContext); s != nil {
	// 	op1 := getValue(s, 0)
	// 	op2 := getValue(s, 1)

	// 	if op := s.LessThan(); op != nil {
	// 		return b.EmitBinOp(ssa.OpLt, op1, op2)
	// 	} else if op := s.MoreThan(); op != nil {
	// 		return b.EmitBinOp(ssa.OpGt, op1, op2)
	// 	} else if op := s.LessThanEquals(); op != nil {
	// 		return b.EmitBinOp(ssa.OpLtEq, op1, op2)
	// 	} else if op := s.GreaterThanEquals(); op != nil {
	// 		return b.EmitBinOp(ssa.OpGtEq, op1, op2)
	// 	} else {
	// 		b.NewError(ssa.Error, TAG, "binary operator not support: %s", op.GetText())
	// 		return nil
	// 	}

	// }

	// //unary
	// if s, ok := stmt.(*JS.BitAndExpressionContext); ok {
	// 	result = b.buildBitAndExpression(s)
	// }

	// if s, ok := stmt.(*JS.BitShiftExpressionContext); ok {
	// 	result = b.buildBitShiftExpression(s)
	// }

	// if s, ok := stmt.(*JS.BitOrExpressionContext); ok {
	// 	result = b.buildBitOrExpression(s)
	// }

	// if s, ok := stmt.(*JS.BitXOrExpressionContext); ok {
	// 	result = b.buildBitXOrExpression(s)
	// }

	// if s, ok := stmt.(*JS.BitNotExpressionContext); ok {
	// 	result = b.buildBitNotExpression(s)
	// }

	return result
}

func (b *astbuilder) AssignExpression(val ssa.Value, stmt JS.IVariableDeclarationContext) {
	b.WriteVariable(stmt.Assignable().GetText(), val)
}

// func (b *astbuilder) HandleUnExpressionOperator(value []ssa.Value, optext string) ssa.Value {
// 	var UnOpcode ssa.UnaryOpcode
// 	var BinOpcode ssa.BinaryOpcode

// 	if strings.Contains(optext, "^"){
// 		UnOpcode = ssa.OpBitwiseNot
// 	} else if strings.Contains(optext, "&"){
// 		// opcode = ssa.UnaryOpcode(ssa.OpAnd)
// 	} else if strings.Contains(optext, "|"){
// 		// opcode = ssa.OpMod
// 	} else if strings.Contains(optext, "<<"){
// 		// opcode = ssa.OpMod
// 	} else if strings.Contains(optext, ">>"){
// 		// opcode = ssa.OpMod
// 	} else if strings.Contains(optext, ">>"){
// 		// opcode = ssa.OpMod
// 	}

// 	if UnOpcode != 0 {
// 		return  b.EmitUnOp(UnOpcode, value[0], value[1])
// 	} else if BinOpcode != 0 {
// 		return b.EmitBinOp(BinOpcode, value[0], value[1])
// 	} else {
// 		b.NewError(ssa.Error, TAG, "binary operator not support: %s", optext)
// 		return nil
// 	}
// }

// func (b *astbuilder) HandleBinExpressionOperator(value []ssa.Value, optext string) ssa.Value {
// 	var opcode ssa.BinaryOpcode

// 	// TODO: >>>

// 	if strings.Contains(optext, "+"){
// 		opcode = ssa.OpAdd
// 	} else if strings.Contains(optext, "-"){
// 		opcode = ssa.OpSub
// 	} else if strings.Contains(optext, "*"){
// 		opcode = ssa.OpMul
// 	} else if strings.Contains(optext, "/"){
// 		opcode = ssa.OpDiv
// 	} else if strings.Contains(optext, "%"){
// 		opcode = ssa.OpMod
// 	} else if strings.Contains(optext, "^"){
// 		opcode = ssa.OpXor
// 	} else if strings.Contains(optext, "&"){
// 		opcode = ssa.OpAnd
// 	} else if strings.Contains(optext, "|"){
// 		opcode = ssa.OpOr
// 	} else if strings.Contains(optext, "<<"){
// 		opcode = ssa.OpShl
// 	} else if strings.Contains(optext, ">>"){
// 		opcode = ssa.OpShr
// 	} else {
// 		b.NewError(ssa.Error, TAG, "binary operator not support: %s", optext)
// 		return nil
// 	}

// 	return b.EmitBinOp(opcode, value[0], value[1])
// }

// func (b *astbuilder) buildAdditiveExpression(stmt *JS.AdditiveExpressionContext) ssa.Value{
// 	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
// 	defer recoverRange()

// 	var value []ssa.Value

// 	optext := stmt.GetText()

// 	for _, single := range stmt.AllSingleExpression() {
// 		val := b.buildSingleExpression(single)
// 		value = append(value, val)
// 	}

// 	return b.HandleBinExpressionOperator(value, optext)
// }

// func (b *astbuilder) buildMultiplicativeExpression(stmt *JS.MultiplicativeExpressionContext) ssa.Value {
// 	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
// 	defer recoverRange()

// 	var value []ssa.Value

// 	optext := stmt.GetText()

// 	for _, single := range stmt.AllSingleExpression() {
// 		val := b.buildSingleExpression(single)
// 		value = append(value, val)
// 	}

// 	return b.HandleBinExpressionOperator(value, optext)
// }

// func (b *astbuilder) buildBitAndExpression(stmt *JS.BitAndExpressionContext) ssa.Value {
// 	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
// 	defer recoverRange()

// 	var value []ssa.Value

// 	optext := stmt.GetText()

// 	for _, single := range stmt.AllSingleExpression() {
// 		val := b.buildSingleExpression(single)
// 		value = append(value, val)
// 	}

// 	return b.HandleBinExpressionOperator(value, optext)
// }

// func (b *astbuilder) buildBitShiftExpression(stmt *JS.BitShiftExpressionContext) ssa.Value {
// 	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
// 	defer recoverRange()

// 	var value []ssa.Value

// 	optext := stmt.GetText()

// 	for _, single := range stmt.AllSingleExpression() {
// 		val := b.buildSingleExpression(single)
// 		value = append(value, val)
// 	}

// 	return b.HandleBinExpressionOperator(value, optext)
// }

// func (b *astbuilder) buildBitXOrExpression(stmt *JS.BitXOrExpressionContext) ssa.Value {
// 	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
// 	defer recoverRange()

// 	var value []ssa.Value

// 	optext := stmt.GetText()

// 	for _, single := range stmt.AllSingleExpression() {
// 		val := b.buildSingleExpression(single)
// 		value = append(value, val)
// 	}

// 	return b.HandleBinExpressionOperator(value, optext)
// }

// func (b *astbuilder) buildBitOrExpression(stmt *JS.BitOrExpressionContext) ssa.Value {
// 	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
// 	defer recoverRange()

// 	var value []ssa.Value

// 	optext := stmt.GetText()

// 	for _, single := range stmt.AllSingleExpression() {
// 		val := b.buildSingleExpression(single)
// 		value = append(value, val)
// 	}

// 	return b.HandleBinExpressionOperator(value, optext)
// }

// func (b *astbuilder) buildBitNotExpression(stmt *JS.BitNotExpressionContext) ssa.Value {
// 	recoverRange := b.SetRange(&stmt.BaseParserRuleContext)
// 	defer recoverRange()

// 	var opcode ssa.UnaryOpcode

// 	optext := stmt.GetText()
// 	single := stmt.SingleExpression()

// 	val := b.buildSingleExpression(single)

// 	if strings.Contains(optext, "!") {
// 		opcode = ssa.OpNot
// 	} else {
// 		b.NewError(ssa.Error, TAG, "unary operator not support: %s", stmt.GetText())
// 		return nil
// 	}

// 	return b.EmitUnOp(opcode, val)

// }

func (b *astbuilder) buildExpressionStatement(*JS.ExpressionStatementContext) {

}
