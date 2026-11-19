package rust2ssa

import (
	rustparser "github.com/yaklang/yaklang/common/yak/rust/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) visitBlockExpression(raw rustparser.IBlockExpressionContext, functionBody bool) ssa.Value {
	block, ok := raw.(*rustparser.BlockExpressionContext)
	if !ok || block == nil {
		return nil
	}
	statements := block.Statements()
	if statements == nil {
		return nil
	}
	return b.visitStatements(statements, functionBody)
}

func (b *singleFileBuilder) visitStatements(raw rustparser.IStatementsContext, functionBody bool) ssa.Value {
	statements, ok := raw.(*rustparser.StatementsContext)
	if !ok || statements == nil {
		return nil
	}
	for _, rawStatement := range statements.AllStatement() {
		b.visitStatement(rawStatement)
		if b.IsStop() {
			return nil
		}
	}
	if trailing := statements.Expression(); trailing != nil {
		value := b.VisitExpression(trailing)
		if functionBody && value != nil {
			b.EmitReturn([]ssa.Value{value})
		}
		return value
	}
	return nil
}

func (b *singleFileBuilder) visitStatement(raw rustparser.IStatementContext) {
	statement, ok := raw.(*rustparser.StatementContext)
	if !ok || statement == nil {
		return
	}
	recoverRange := b.SetRange(statement)
	defer recoverRange()

	switch {
	case statement.SEMI() != nil:
		return
	case statement.Item() != nil:
		b.visitItem(statement.Item())
	case statement.LetStatement() != nil:
		b.visitLet(statement.LetStatement())
	case statement.ExpressionStatement() != nil:
		b.visitExpressionStatement(statement.ExpressionStatement())
	case statement.MacroInvocationSemi() != nil:
		return
	}
}

func (b *singleFileBuilder) visitLet(raw rustparser.ILetStatementContext) {
	let, ok := raw.(*rustparser.LetStatementContext)
	if !ok || let == nil {
		return
	}
	name := rustPatternNoTopAltName(let.PatternNoTopAlt())
	if name == "" {
		return
	}
	var value ssa.Value = b.EmitConstInstNil()
	if let.Expression() != nil {
		if v := b.VisitExpression(let.Expression()); v != nil {
			value = v
		}
	}
	b.AssignVariable(b.CreateLocalVariable(name), value)
}

func (b *singleFileBuilder) visitExpressionStatement(raw rustparser.IExpressionStatementContext) {
	statement, ok := raw.(*rustparser.ExpressionStatementContext)
	if !ok || statement == nil {
		return
	}
	if statement.Expression() != nil {
		b.VisitExpression(statement.Expression())
		return
	}
	if statement.ExpressionWithBlock() != nil {
		b.visitExpressionWithBlock(statement.ExpressionWithBlock())
	}
}
