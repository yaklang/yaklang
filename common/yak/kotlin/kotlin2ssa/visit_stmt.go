package kotlin2ssa

import (
	"fmt"
	"strings"
	"sync/atomic"

	kotlinparser "github.com/yaklang/yaklang/common/yak/kotlin/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

var kotlinIfExpressionSeq atomic.Uint64

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
		name := direct.SimpleIdentifier().GetText()
		b.AssignVariable(b.CreateVariable(name), value)
		if b.classFields != nil {
			if _, ok := b.classFields[name]; ok {
				b.classFields[name] = value
			}
		}
		return
	}
	if direct.PostfixUnaryExpression() == nil {
		return
	}
	isThis := strings.HasPrefix(direct.PostfixUnaryExpression().GetText(), "this.")
	if postfix, ok := direct.PostfixUnaryExpression().(*kotlinparser.PostfixUnaryExpressionContext); ok && postfix != nil {
		if primary, ok := postfix.PrimaryExpression().(*kotlinparser.PrimaryExpressionContext); ok && primary != nil {
			isThis = isThis || primary.ThisExpression() != nil
		}
	}
	base := b.visitPostfixUnaryExpression(direct.PostfixUnaryExpression())
	if isThis || (base != nil && base.GetName() == "this") {
		if suffix := direct.AssignableSuffix(); suffix != nil {
			b.assignThisSuffix(suffix, value)
			return
		}
	}
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

func (b *singleFileBuilder) assignThisSuffix(raw kotlinparser.IAssignableSuffixContext, value ssa.Value) {
	suffix, ok := raw.(*kotlinparser.AssignableSuffixContext)
	if !ok || suffix == nil || suffix.NavigationSuffix() == nil {
		return
	}
	nav, ok := suffix.NavigationSuffix().(*kotlinparser.NavigationSuffixContext)
	if !ok || nav == nil || nav.SimpleIdentifier() == nil {
		return
	}
	name := nav.SimpleIdentifier().GetText()
	b.AssignVariable(b.CreateVariable(name), value)
	if b.classFields != nil {
		if _, ok := b.classFields[name]; ok {
			b.classFields[name] = value
		}
	}
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

func (b *singleFileBuilder) visitIfExpression(raw kotlinparser.IIfExpressionContext) ssa.Value {
	ifExpr, ok := raw.(*kotlinparser.IfExpressionContext)
	if !ok || ifExpr == nil || ifExpr.Expression() == nil {
		return b.EmitConstInstNil()
	}
	name := fmt.Sprintf("__kotlin_if_%d", kotlinIfExpressionSeq.Add(1))
	b.AssignVariable(b.CreateLocalVariable(name), b.EmitConstInstNil())
	assign := func(body func() ssa.Value) {
		value := body()
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateVariable(name), value)
	}
	ifBuilder := b.CreateIfBuilder()
	ifBuilder.SetCondition(func() ssa.Value {
		return b.VisitExpression(ifExpr.Expression())
	}, func() {
		assign(func() ssa.Value {
			return b.visitControlStructureBodyValue(ifExpr.ControlStructureBody(0))
		})
	})
	if ifExpr.ELSE() != nil && ifExpr.ControlStructureBody(1) != nil {
		ifBuilder.SetElse(func() {
			assign(func() ssa.Value {
				return b.visitControlStructureBodyValue(ifExpr.ControlStructureBody(1))
			})
		})
	}
	ifBuilder.Build()
	return b.ReadValue(name)
}

// visitBlockValue returns the value of the last statement so lambdas and
// expression bodies keep their result.
func (b *singleFileBuilder) visitBlockValue(raw kotlinparser.IBlockContext) ssa.Value {
	block, ok := raw.(*kotlinparser.BlockContext)
	if !ok || block == nil || block.Statements() == nil {
		return nil
	}
	return b.visitStatementsValue(block.Statements())
}

func (b *singleFileBuilder) visitStatementsValue(raw kotlinparser.IStatementsContext) ssa.Value {
	statements, ok := raw.(*kotlinparser.StatementsContext)
	if !ok || statements == nil {
		return nil
	}
	var result ssa.Value
	for _, rawStatement := range statements.AllStatement() {
		result = b.visitStatementValue(rawStatement)
		if b.IsStop() {
			return result
		}
	}
	return result
}

func (b *singleFileBuilder) visitStatementValue(raw kotlinparser.IStatementContext) ssa.Value {
	statement, ok := raw.(*kotlinparser.StatementContext)
	if !ok || statement == nil {
		return nil
	}
	switch {
	case statement.Expression() != nil:
		return b.VisitExpression(statement.Expression())
	case statement.Assignment() != nil:
		b.visitAssignment(statement.Assignment())
		return nil
	default:
		b.visitStatement(raw)
		return nil
	}
}

func (b *singleFileBuilder) visitControlStructureBodyValue(raw kotlinparser.IControlStructureBodyContext) ssa.Value {
	body, ok := raw.(*kotlinparser.ControlStructureBodyContext)
	if !ok || body == nil {
		return nil
	}
	if body.Block() != nil {
		return b.visitBlockValue(body.Block())
	}
	if body.Statement() != nil {
		return b.visitStatementValue(body.Statement())
	}
	return nil
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
