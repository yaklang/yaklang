package kotlin2ssa

import (
	"strings"

	kotlinparser "github.com/yaklang/yaklang/common/yak/kotlin/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) visitBlock(raw kotlinparser.IBlockContext) {
	block, ok := raw.(*kotlinparser.BlockContext)
	if !ok || block == nil || block.Statements() == nil {
		return
	}
	statements, ok := block.Statements().(*kotlinparser.StatementsContext)
	if !ok || statements == nil {
		return
	}
	for _, rawStatement := range statements.AllStatement() {
		b.visitStatement(rawStatement)
		if b.IsStop() {
			return
		}
	}
}

func (b *singleFileBuilder) visitStatement(raw kotlinparser.IStatementContext) {
	statement, ok := raw.(*kotlinparser.StatementContext)
	if !ok || statement == nil {
		return
	}
	recoverRange := b.SetRange(statement)
	defer recoverRange()

	switch {
	case statement.Declaration() != nil:
		b.visitDeclaration(statement.Declaration())
	case statement.Assignment() != nil:
		b.visitAssignment(statement.Assignment())
	case statement.LoopStatement() != nil:
		b.visitLoopStatement(statement.LoopStatement())
	case statement.Expression() != nil:
		b.VisitExpression(statement.Expression())
	}
}

func (b *singleFileBuilder) visitAssignment(raw kotlinparser.IAssignmentContext) {
	assignment, ok := raw.(*kotlinparser.AssignmentContext)
	if !ok || assignment == nil || assignment.Expression() == nil {
		return
	}
	value := b.VisitExpression(assignment.Expression())
	if value == nil {
		value = b.EmitConstInstNil()
	}
	if direct := assignment.DirectlyAssignableExpression(); direct != nil {
		b.assignDirectly(direct, value)
		return
	}
	if assignable := assignment.AssignableExpression(); assignable != nil {
		b.assignAssignable(assignable, assignment.AssignmentAndOperator(), value)
	}
}

func (b *singleFileBuilder) assignDirectly(raw kotlinparser.IDirectlyAssignableExpressionContext, value ssa.Value) {
	direct, ok := raw.(*kotlinparser.DirectlyAssignableExpressionContext)
	if !ok || direct == nil {
		return
	}
	if direct.SimpleIdentifier() != nil {
		b.AssignVariable(b.CreateVariable(direct.SimpleIdentifier().GetText()), value)
		return
	}
	if direct.PostfixUnaryExpression() == nil {
		return
	}
	base := b.visitPostfixUnaryExpression(direct.PostfixUnaryExpression())
	if suffix := direct.AssignableSuffix(); suffix != nil {
		b.assignSuffix(base, suffix, value)
		return
	}
	if base != nil {
		b.AssignVariable(b.CreateVariable(direct.PostfixUnaryExpression().GetText()), value)
	}
}

func (b *singleFileBuilder) assignAssignable(raw kotlinparser.IAssignableExpressionContext, operator kotlinparser.IAssignmentAndOperatorContext, value ssa.Value) {
	assignable, ok := raw.(*kotlinparser.AssignableExpressionContext)
	if !ok || assignable == nil || assignable.PrefixUnaryExpression() == nil {
		return
	}
	current := b.visitPrefixUnaryExpression(assignable.PrefixUnaryExpression())
	if current == nil {
		current = b.EmitConstInstNil()
	}
	op := ""
	if operator != nil {
		op = operator.GetText()
	}
	op = strings.TrimSuffix(op, "=")
	result := b.EmitBinOp(kotlinBinaryOpcode(op), current, value)
	b.AssignVariable(b.CreateVariable(assignable.PrefixUnaryExpression().GetText()), result)
}

func (b *singleFileBuilder) assignSuffix(base ssa.Value, raw kotlinparser.IAssignableSuffixContext, value ssa.Value) {
	suffix, ok := raw.(*kotlinparser.AssignableSuffixContext)
	if !ok || suffix == nil {
		return
	}
	if suffix.NavigationSuffix() != nil {
		if nav, ok := suffix.NavigationSuffix().(*kotlinparser.NavigationSuffixContext); ok && nav != nil && nav.SimpleIdentifier() != nil {
			if base == nil {
				base = b.EmitUndefined("receiver")
			}
			key := b.EmitConstInst(nav.SimpleIdentifier().GetText())
			b.AssignVariable(b.CreateMemberCallVariable(base, key), value)
		}
		return
	}
	if suffix.IndexingSuffix() != nil {
		if index, ok := suffix.IndexingSuffix().(*kotlinparser.IndexingSuffixContext); ok && index != nil && len(index.AllExpression()) > 0 {
			if base == nil {
				base = b.EmitUndefined("receiver")
			}
			key := b.VisitExpression(index.Expression(0))
			if key != nil {
				b.AssignVariable(b.CreateMemberCallVariable(base, key), value)
			}
		}
	}
}

func (b *singleFileBuilder) visitLoopStatement(raw kotlinparser.ILoopStatementContext) {
	loop, ok := raw.(*kotlinparser.LoopStatementContext)
	if !ok || loop == nil {
		return
	}
	switch {
	case loop.ForStatement() != nil:
		b.visitForStatement(loop.ForStatement())
	case loop.WhileStatement() != nil:
		b.visitWhileStatement(loop.WhileStatement())
	case loop.DoWhileStatement() != nil:
		b.visitDoWhileStatement(loop.DoWhileStatement())
	}
}

func (b *singleFileBuilder) visitForStatement(raw kotlinparser.IForStatementContext) {
	forCtx, ok := raw.(*kotlinparser.ForStatementContext)
	if !ok || forCtx == nil {
		return
	}
	name := ""
	if forCtx.VariableDeclaration() != nil {
		if variable, ok := forCtx.VariableDeclaration().(*kotlinparser.VariableDeclarationContext); ok && variable.SimpleIdentifier() != nil {
			name = variable.SimpleIdentifier().GetText()
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
		b.visitControlStructureBody(forCtx.ControlStructureBody())
	})
	loop.Finish()
}

func (b *singleFileBuilder) visitWhileStatement(raw kotlinparser.IWhileStatementContext) {
	while, ok := raw.(*kotlinparser.WhileStatementContext)
	if !ok || while == nil {
		return
	}
	loop := b.CreateLoopBuilder()
	loop.SetCondition(func() ssa.Value {
		return b.VisitExpression(while.Expression())
	})
	loop.SetBody(func() {
		b.visitControlStructureBody(while.ControlStructureBody())
	})
	loop.Finish()
}

func (b *singleFileBuilder) visitDoWhileStatement(raw kotlinparser.IDoWhileStatementContext) {
	doWhile, ok := raw.(*kotlinparser.DoWhileStatementContext)
	if !ok || doWhile == nil {
		return
	}
	loop := b.CreateLoopBuilder()
	loop.SetCondition(func() ssa.Value { return b.EmitConstInst(true) })
	loop.SetBody(func() {
		b.visitControlStructureBody(doWhile.ControlStructureBody())
		if doWhile.Expression() == nil {
			return
		}
		ifBuilder := b.CreateIfBuilder()
		ifBuilder.SetCondition(func() ssa.Value {
			return b.VisitExpression(doWhile.Expression())
		}, func() {
			b.Break()
		})
		ifBuilder.Build()
	})
	loop.Finish()
}

func (b *singleFileBuilder) visitControlStructureBody(raw kotlinparser.IControlStructureBodyContext) {
	body, ok := raw.(*kotlinparser.ControlStructureBodyContext)
	if !ok || body == nil {
		return
	}
	if body.Block() != nil {
		b.visitBlock(body.Block())
		return
	}
	if body.Statement() != nil {
		b.visitStatement(body.Statement())
	}
}

func (b *singleFileBuilder) visitIfExpression(raw kotlinparser.IIfExpressionContext) {
	ifExpr, ok := raw.(*kotlinparser.IfExpressionContext)
	if !ok || ifExpr == nil || ifExpr.Expression() == nil {
		return
	}
	ifBuilder := b.CreateIfBuilder()
	ifBuilder.SetCondition(func() ssa.Value {
		return b.VisitExpression(ifExpr.Expression())
	}, func() {
		if ifExpr.ControlStructureBody(0) != nil {
			b.visitControlStructureBody(ifExpr.ControlStructureBody(0))
		}
	})
	if ifExpr.ELSE() != nil && ifExpr.ControlStructureBody(1) != nil {
		ifBuilder.SetElse(func() {
			b.visitControlStructureBody(ifExpr.ControlStructureBody(1))
		})
	}
	ifBuilder.Build()
}

func (b *singleFileBuilder) visitJumpExpression(raw kotlinparser.IJumpExpressionContext) {
	jump, ok := raw.(*kotlinparser.JumpExpressionContext)
	if !ok || jump == nil {
		return
	}
	switch {
	case jump.RETURN() != nil || jump.RETURN_AT() != nil:
		values := []ssa.Value{}
		if jump.Expression() != nil {
			if value := b.VisitExpression(jump.Expression()); value != nil {
				values = append(values, value)
			}
		}
		b.EmitReturn(values)
	case jump.BREAK() != nil || jump.BREAK_AT() != nil:
		b.Break()
	case jump.CONTINUE() != nil || jump.CONTINUE_AT() != nil:
		b.Continue()
	case jump.THROW() != nil && jump.Expression() != nil:
		b.VisitExpression(jump.Expression())
	}
}
