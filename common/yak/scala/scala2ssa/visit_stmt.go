package scala2ssa

import (
	scalaparser "github.com/yaklang/yaklang/common/yak/scala/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) VisitExpr(raw scalaparser.IExprContext) ssa.Value {
	expr, ok := raw.(*scalaparser.ExprContext)
	if !ok || expr == nil {
		return b.EmitConstInstNil()
	}
	if expr.Expr1() != nil {
		return b.VisitExpr1(expr.Expr1())
	}
	if expr.Expr() != nil {
		return b.VisitExpr(expr.Expr())
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) VisitExpr1(raw scalaparser.IExpr1Context) ssa.Value {
	expr, ok := raw.(*scalaparser.Expr1Context)
	if !ok || expr == nil {
		return b.EmitConstInstNil()
	}
	switch {
	case expr.IF() != nil:
		return b.visitIfExpr1(expr)
	case expr.WHILE() != nil:
		return b.visitWhileExpr1(expr)
	case expr.RETURN() != nil:
		values := []ssa.Value{}
		if len(expr.AllExpr()) > 0 {
			if value := b.VisitExpr(expr.Expr(0)); value != nil {
				values = append(values, value)
			}
		}
		b.EmitReturn(values)
		return b.EmitConstInstNil()
	case expr.THROW() != nil:
		if len(expr.AllExpr()) > 0 {
			b.VisitExpr(expr.Expr(0))
		}
		return b.EmitConstInstNil()
	case expr.ForExpr() != nil:
		b.visitForExpr(expr.ForExpr())
		return b.EmitConstInstNil()
	case expr.ASSIGN() != nil:
		return b.visitAssignExpr1(expr)
	case expr.PostfixExpr() != nil:
		return b.visitPostfixExpr(expr.PostfixExpr())
	case expr.InfixExpr() != nil:
		return b.visitInfixExpr(expr.InfixExpr())
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitIfExpr1(expr *scalaparser.Expr1Context) ssa.Value {
	exprs := expr.AllExpr()
	if len(exprs) < 2 {
		return b.EmitConstInstNil()
	}
	ifBuilder := b.CreateIfBuilder()
	ifBuilder.SetCondition(func() ssa.Value {
		return b.VisitExpr(exprs[0])
	}, func() {
		b.VisitExpr(exprs[1])
	})
	if expr.ELSE() != nil && len(exprs) > 2 {
		ifBuilder.SetElse(func() {
			b.VisitExpr(exprs[2])
		})
	}
	ifBuilder.Build()
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitWhileExpr1(expr *scalaparser.Expr1Context) ssa.Value {
	exprs := expr.AllExpr()
	if len(exprs) < 2 {
		return b.EmitConstInstNil()
	}
	loop := b.CreateLoopBuilder()
	loop.SetCondition(func() ssa.Value { return b.VisitExpr(exprs[0]) })
	loop.SetBody(func() { b.VisitExpr(exprs[1]) })
	loop.Finish()
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitAssignExpr1(expr *scalaparser.Expr1Context) ssa.Value {
	var value ssa.Value = b.EmitConstInstNil()
	if len(expr.AllExpr()) > 0 {
		if v := b.VisitExpr(expr.Expr(0)); v != nil {
			value = v
		}
	}
	if expr.PostfixExpr() != nil {
		target := expr.PostfixExpr().GetText()
		b.AssignVariable(b.CreateVariable(target), value)
	}
	return value
}

func (b *singleFileBuilder) visitForExpr(raw scalaparser.IForExprContext) {
	forExpr, ok := raw.(*scalaparser.ForExprContext)
	if !ok || forExpr == nil {
		return
	}
	name := ""
	if enumerators := forExpr.Enumerators0(); enumerators != nil {
		if list, ok := enumerators.(*scalaparser.Enumerators0Context); ok && list != nil && list.Enumerators() != nil {
			if values, ok := list.Enumerators().(*scalaparser.EnumeratorsContext); ok && values != nil {
				for _, rawEnumerator := range values.AllEnumerator() {
					enumerator, ok := rawEnumerator.(*scalaparser.EnumeratorContext)
					if !ok || enumerator == nil || enumerator.Generator() == nil {
						continue
					}
					generator, ok := enumerator.Generator().(*scalaparser.GeneratorContext)
					if !ok || generator == nil {
						continue
					}
					if generator.Pattern1() != nil {
						name = generator.Pattern1().GetText()
					}
					if generator.Expr() != nil {
						b.VisitExpr(generator.Expr())
					}
					break
				}
			}
		}
	}
	loop := b.CreateLoopBuilder()
	loop.SetFirst(func() []ssa.Value {
		if name != "" {
			b.AssignVariable(b.CreateLocalVariable(name), b.EmitUndefined(name))
		}
		return nil
	})
	loop.SetCondition(func() ssa.Value { return b.EmitConstInst(true) })
	loop.SetBody(func() {
		if forExpr.Expr() != nil {
			b.VisitExpr(forExpr.Expr())
			return
		}
		for _, rawStat := range forExpr.AllBlockStat() {
			stat, ok := rawStat.(*scalaparser.BlockStatContext)
			if !ok || stat == nil {
				continue
			}
			if stat.Def_() != nil {
				b.visitDef(stat.Def_())
			} else if stat.Expr1() != nil {
				b.VisitExpr1(stat.Expr1())
			}
		}
	})
	loop.Finish()
}
