//go:build !no_language
// +build !no_language

package php2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitIfStatement(raw phpparser.IIfStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.IfStatementContext)
	if i == nil {
		return nil
	}
	stmt := i
	handlerIfStatement := func(elseBody func(), items ...ssa.IfBuilderItem) interface{} {
		ifBuilder := y.CreateIfBuilder()
		for _, item := range items {
			ifBuilder.AppendItem(item.Condition, item.Body)
		}
		if elseBody != nil {
			ifBuilder.SetElse(elseBody)
		}
		ifBuilder.Build()
		return nil
	}
	var elseBody func()
	var items []ssa.IfBuilderItem
	if i.Colon() == nil {
		if i.ElseStatement() != nil {
			elseBody = func() {
				y.VisitElseStatement(i.ElseStatement())
			}
		}
		//先放入第一个if的condition和body
		items = append(items, ssa.IfBuilderItem{
			Condition: func() ssa.Value {
				return y.VisitParentheses(i.Parentheses())
			},
			Body: func() {
				y.VisitStatement(i.Statement())
			},
		})
		//再放入所有else-if的condition和body
		for _, statement := range stmt.AllElseIfStatement() {
			if stmtItem, ok := y.VisitElseIfStatement(statement).(ssa.IfBuilderItem); !ok {
				continue
			} else {
				items = append(items, stmtItem)
			}
		}
		//说明是第二种情况
	} else if i.EndIf() != nil && i.SemiColon() != nil {
		elseBody = func() {
			y.VisitElseColonStatement(i.ElseColonStatement())
		}
		items = append(items, ssa.IfBuilderItem{
			Condition: func() ssa.Value {
				return y.VisitParentheses(stmt.Parentheses())
			},
			Body: func() {
				y.VisitInnerStatementList(stmt.InnerStatementList())
			},
		})
		for _, statement := range stmt.AllElseIfColonStatement() {
			if item, ok := y.VisitElseIfColonStatement(statement).(ssa.IfBuilderItem); !ok {
				return nil
			} else {
				items = append(items, item)
			}
		}
	} else {
		return nil
	}
	return handlerIfStatement(elseBody, items...)
}

func (y *builder) VisitElseIfStatement(raw phpparser.IElseIfStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	var stmt *phpparser.ElseIfStatementContext
	if elseIfStatement, _ := raw.(*phpparser.ElseIfStatementContext); elseIfStatement == nil {
		return nil
	} else {
		stmt = elseIfStatement
	}
	return ssa.IfBuilderItem{
		Condition: func() ssa.Value {
			return y.VisitParentheses(stmt.Parentheses())
		},
		Body: func() {
			y.VisitStatement(stmt.Statement())
		},
	}
}
func (y *builder) VisitElseIfColonStatement(raw phpparser.IElseIfColonStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ElseIfColonStatementContext)
	if i == nil {
		return nil
	}
	return ssa.IfBuilderItem{
		Condition: func() ssa.Value {
			return y.VisitParentheses(i.Parentheses())
		},
		Body: func() {
			y.VisitInnerStatementList(i.InnerStatementList())
		},
	}
}
func (y *builder) VisitElseStatement(raw phpparser.IElseStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	statement, _ := raw.(*phpparser.ElseStatementContext)
	if statement == nil {
		return nil
	}
	return y.VisitStatement(statement.Statement())
}

func (y *builder) VisitElseColonStatement(raw phpparser.IElseColonStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	statement, _ := raw.(*phpparser.ElseColonStatementContext)
	if statement == nil {
		return nil
	}
	return y.VisitInnerStatementList(statement.InnerStatementList())
}

func (y *builder) VisitSwitchStatement(raw phpparser.ISwitchStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	stmt, _ := raw.(*phpparser.SwitchStatementContext)
	if stmt == nil {
		return nil
	}
	if len(stmt.AllSwitchDefaultBlock()) > 1 {
		log.Printf("switch default number illegal")
		return nil
	}
	ir := y.BuildSwitch()
	ir.AutoBreak = false
	ir.BuildCondition(func() ssa.Value {
		return y.VisitParentheses(stmt.Parentheses())
	})
	if len(stmt.AllSwitchDefaultBlock()) > 0 {
		ir.BuildDefault(y.VisitSwitchDefaultBlock(stmt.SwitchDefaultBlock(0)))
	}
	ir.BuildCaseSize(len(stmt.AllSwitchCaseBlock()))
	ir.SetCase(func(i int) []ssa.Value {
		block, _ := y.VisitSwitchCaseBlock(stmt.SwitchCaseBlock(i))
		return block()
	})
	ir.BuildBody(func(i int) {
		_, f := y.VisitSwitchCaseBlock(stmt.SwitchCaseBlock(i))
		f()
	})
	ir.Finish()
	return nil
}

func (y *builder) VisitSwitchCaseBlock(raw phpparser.ISwitchCaseBlockContext) (func() []ssa.Value, func()) {
	if y == nil || raw == nil || y.IsStop() {
		return nil, nil
	}
	stmt, _ := raw.(*phpparser.SwitchCaseBlockContext)
	if stmt == nil {
		return nil, nil
	}
	return func() []ssa.Value {
			return []ssa.Value{y.VisitExpression(stmt.Expression())}
		}, func() {
			y.VisitInnerStatementList(stmt.InnerStatementList())
		}
}

func (y *builder) VisitSwitchDefaultBlock(raw phpparser.ISwitchDefaultBlockContext) func() {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	stmt, _ := raw.(*phpparser.SwitchDefaultBlockContext)
	if stmt == nil {
		return nil
	}
	return func() {
		y.VisitInnerStatementList(stmt.InnerStatementList())
	}
}
