package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitIfStatement(raw phpparser.IIfStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.IfStatementContext)
	if i == nil {
		return nil
	}
	stmt := i

	if i.Colon() == nil {
		// classicIf
		/*
			if (true) echo "abc";
			if (true) echo "abc"; else if true 1+1;
			if (true) echo "abc"; else if true 1+1; else "abc"."ccc";
		*/
		i := y.ir.BuildIf()
		i.BuildCondition(func() ssa.Value {
			return y.VisitParentheses(stmt.Parentheses())
		})
		i.BuildTrue(func() {
			y.VisitStatement(stmt.Statement())
		})
		for _, elseIf := range stmt.AllElseIfStatement() {
			elseIfStmt := elseIf.(*phpparser.ElseIfStatementContext)
			i.BuildElif(func() ssa.Value {
				return y.VisitParentheses(elseIfStmt.Parentheses())
			}, func() {
				y.VisitStatement(elseIfStmt.Statement())
			})
		}
		if stmt.ElseStatement() != nil {
			i.BuildFalse(func() {
				y.VisitStatement(stmt.ElseStatement().(*phpparser.ElseStatementContext).Statement())
			})
		}
		i.Finish()
	} else {
		// tag if
		i := y.ir.BuildIf()
		i.BuildCondition(func() ssa.Value {
			return y.VisitParentheses(stmt.Parentheses())
		})
		i.BuildTrue(func() {
			y.VisitInnerStatementList(stmt.InnerStatementList())
		})
		for _, elseIf := range stmt.AllElseIfColonStatement() {
			elseIfStmt := elseIf.(*phpparser.ElseIfColonStatementContext)
			i.BuildElif(func() ssa.Value {
				return y.VisitParentheses(elseIfStmt.Parentheses())
			}, func() {
				y.VisitInnerStatementList(elseIfStmt.InnerStatementList())
			})
		}
		if stmt.ElseStatement() != nil {
			i.BuildFalse(func() {
				y.VisitInnerStatementList(stmt.ElseColonStatement().(*phpparser.ElseColonStatementContext).InnerStatementList())
			})
		}
		i.Finish()
	}

	return nil
}

func (y *builder) VisitSwitchStatement(raw phpparser.ISwitchStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.SwitchStatementContext)
	if i == nil {
		return nil
	}

	ir := y.ir.BuildSwitch()
	ir.DefaultBreak = false

	var cond ssa.Value
	ir.BuildCondition(func() ssa.Value {
		cond = y.VisitParentheses(i.Parentheses())
		return cond
	})
	blocks := i.AllSwitchBlock()
	var results = make([]ssa.Value, len(blocks))
	_ = results
	return nil
}
