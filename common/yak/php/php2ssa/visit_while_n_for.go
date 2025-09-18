package php2ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitWhileStatement(raw phpparser.IWhileStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.WhileStatementContext)
	if i == nil {
		return nil
	}
	loopBuilder := y.CreateLoopBuilder()
	loopBuilder.SetCondition(func() ssa.Value {
		return y.VisitParentheses(i.Parentheses())
	})
	loopBuilder.SetBody(func() {
		if i.Colon() != nil {
			y.VisitInnerStatementList(i.InnerStatementList())
		} else {
			y.VisitStatement(i.Statement())
		}
	})
	loopBuilder.Finish()
	return nil
}

func (y *builder) VisitDoWhileStatement(raw phpparser.IDoWhileStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.DoWhileStatementContext)
	if i == nil {
		return nil
	}
	loopBuilder := y.CreateLoopBuilder()
	_ = loopBuilder
	loopBuilder.SetCondition(func() ssa.Value {
		return y.EmitConstInst(true)
	})
	loopBuilder.SetBody(func() {
		y.VisitStatement(i.Statement())
		y.CreateIfBuilder().SetCondition(func() ssa.Value {
			return y.VisitParentheses(i.Parentheses())
		}, func() {}).SetElse(func() {
			y.Break()
		}).Build()
	})
	loopBuilder.Finish()
	return nil
}

func (y *builder) VisitForStatement(raw phpparser.IForStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ForStatementContext)
	if i == nil {
		return nil
	}
	loopBuilder := y.CreateLoopBuilder()
	if i.ForInit() != nil {
		loopBuilder.SetFirst(func() []ssa.Value {
			return y.VisitForInit(i.ForInit())
		})
	}
	//先设置为true，在if-body前面做匹配
	loopBuilder.SetCondition(func() ssa.Value {
		return y.EmitConstInst(true)
	})
	if i.ForUpdate() != nil {
		loopBuilder.SetThird(func() []ssa.Value {
			return y.VisitForUpdate(i.ForUpdate())
		})
	}
	loopBuilder.SetBody(func() {
		if i.ExpressionList() != nil {
			list := y.VisitExpressionList(i.ExpressionList())
			for _, value := range list {
				value1 := value
				y.CreateIfBuilder().SetCondition(func() ssa.Value {
					return value1
				}, func() {
				}).SetElse(func() {
					y.Break()
				})
			}
		}
		if i.Statement() != nil {
			y.VisitStatement(i.Statement())
		} else {
			y.VisitInnerStatementList(i.InnerStatementList())
		}
	})
	loopBuilder.Finish()
	return nil
}

func (y *builder) VisitForInit(raw phpparser.IForInitContext) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ForInitContext)
	if i == nil {
		return nil
	}
	return y.VisitExpressionList(i.ExpressionList())
}

func (y *builder) VisitForUpdate(raw phpparser.IForUpdateContext) []ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ForUpdateContext)
	if i == nil {
		return nil
	}
	return y.VisitExpressionList(i.ExpressionList())
}

func (y *builder) VisitContinueStatement(raw phpparser.IContinueStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ContinueStatementContext)
	if i == nil {
		return nil
	}
	//if t := y.GetContinue(); t != nil {
	//	return y.EmitJump(t)
	//}
	return nil
}

func (y *builder) VisitForeachStatement(raw phpparser.IForeachStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ForeachStatementContext)
	if i == nil {
		return nil
	}
	loop := y.CreateLoopBuilder()
	var value ssa.Value
	if i.Expression() != nil {
		value = y.VisitExpression(i.Expression())
	} else {
		value = y.VisitChain(i.Chain(0))
	}
	loop.SetFirst(func() []ssa.Value {
		return []ssa.Value{value}
	})
	loop.SetCondition(func() ssa.Value {
		var lefts []*ssa.Variable
		var valueLeft *ssa.Variable
		if i.ArrayDestructuring() != nil {
			lefts = y.VisitArrayDestructuring(i.ArrayDestructuring())
		} else if i.Assignable() != nil {
			lefts = y.VisitASsignVariable(i.Assignable())
		} else if i.AssignmentList() != nil {
			//todo:
		}
		if i.Chain(1) != nil {
			valueLeft = y.VisitChainLeft(i.Chain(1))
		}
		//todo: more variable
		key, field, ok := y.EmitNext(value, false)
		if len(lefts) > 0 {
			if valueLeft == nil {
				y.AssignVariable(lefts[0], field)
				ssa.DeleteInst(key)
			} else {
				y.AssignVariable(lefts[0], key)
				y.AssignVariable(valueLeft, field)
			}
		}
		if utils.IsNil(ok) {
			ok = y.EmitConstInst(true)
			// b.NewError(ssa.Warn, TAG, "loop condition expression is nil, default is true")
		}
		return ok
	})
	loop.SetBody(func() {
		if i.Statement() != nil {
			y.VisitStatement(i.Statement())
		} else {
			y.VisitInnerStatementList(i.InnerStatementList())
		}
	})
	loop.Finish()
	return nil
}
func (y *builder) VisitASsignVariable(raw phpparser.IAssignableContext) []*ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.AssignableContext)
	if i == nil {
		return nil
	}
	var arrays []*ssa.Variable
	if i.Chain() != nil {
		arrays = append(arrays, y.VisitChainLeft(i.Chain()))
	} else {
		arrays = append(arrays, y.VisitArrayCreationLeft(i.ArrayCreation())...)
	}
	return arrays
}

func (y *builder) VisitArrayCreationLeft(raw phpparser.IArrayCreationContext) []*ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ArrayCreationContext)
	if i == nil {
		return nil
	}
	var arrays []*ssa.Variable
	arrays = append(arrays, y.VisitArrayItemListLeft(i.ArrayItemList())...)
	return arrays
}
func (y *builder) VisitArrayItemListLeft(raw phpparser.IArrayItemListContext) []*ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ArrayItemListContext)
	if i == nil {
		return nil
	}
	var arrays []*ssa.Variable
	for _, item := range i.AllArrayItem() {
		arrays = append(arrays, y.VisitArrayItemLeft(item))
	}
	return arrays
}

func (y *builder) VisitArrayItemLeft(raw phpparser.IArrayItemContext) *ssa.Variable {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ArrayItemContext)
	if i == nil {
		return nil
	}

	//todo: 没有翻译 =>
	switch ret := i.Expression(0).(type) {
	case *phpparser.VariableExpressionContext:
		return y.VisitLeftVariable(ret.FlexiVariable())
	}
	return y.VisitChainLeft(i.Chain())
}
