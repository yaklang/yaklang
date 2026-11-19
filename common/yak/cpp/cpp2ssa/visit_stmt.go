package cpp2ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	cppparser "github.com/yaklang/yaklang/common/yak/cpp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) visitCompoundStatement(raw cppparser.ICompoundStatementContext) {
	block, ok := raw.(*cppparser.CompoundStatementContext)
	if !ok || block == nil {
		return
	}
	for _, statement := range statementListOf(block) {
		b.visitStatement(statement)
		if b.IsStop() {
			return
		}
	}
}

func (b *singleFileBuilder) visitStatement(raw cppparser.IStatementContext) {
	statement, ok := raw.(*cppparser.StatementContext)
	if !ok || statement == nil {
		return
	}
	switch {
	case statement.LabeledStatement() != nil:
		b.visitLabeledStatement(statement.LabeledStatement())
	case statement.DeclarationStatement() != nil:
		if declaration, ok := statement.DeclarationStatement().(*cppparser.DeclarationStatementContext); ok && declaration != nil {
			if b.lowerCallShapedDeclaration(declaration.BlockDeclaration()) {
				break
			}
			b.visitBlockDeclaration(declaration.BlockDeclaration())
		}
	case statement.CompoundStatement() != nil:
		b.visitCompoundStatement(statement.CompoundStatement())
	case statement.ExpressionStatement() != nil:
		if expression, ok := statement.ExpressionStatement().(*cppparser.ExpressionStatementContext); ok && expression != nil && expression.Expression() != nil {
			b.evalNode(expression.Expression())
		}
	case statement.SelectionStatement() != nil:
		b.visitSelectionStatement(statement.SelectionStatement())
	case statement.IterationStatement() != nil:
		b.visitIterationStatement(statement.IterationStatement())
	case statement.JumpStatement() != nil:
		b.visitJumpStatement(statement.JumpStatement())
	case statement.TryBlock() != nil:
		b.visitTryBlock(statement.TryBlock())
	}
}

// visitLabeledStatement keeps case/default labels for the switch builder; other
// labels are only names, so the labelled statement is visited in place.
func (b *singleFileBuilder) visitLabeledStatement(raw cppparser.ILabeledStatementContext) {
	labeled, ok := raw.(*cppparser.LabeledStatementContext)
	if !ok || labeled == nil {
		return
	}
	if statement := labeled.Statement(); statement != nil {
		b.visitStatement(statement)
	}
}

func (b *singleFileBuilder) visitSelectionStatement(raw cppparser.ISelectionStatementContext) {
	selection, ok := raw.(*cppparser.SelectionStatementContext)
	if !ok || selection == nil {
		return
	}
	switch {
	case selection.If() != nil:
		statements := selection.AllStatement()
		if len(statements) == 0 {
			return
		}
		condition := selection.Condition()
		builder := b.CreateIfBuilder()
		builder.SetCondition(func() ssa.Value {
			return b.visitCondition(condition)
		}, func() {
			b.visitStatement(statements[0])
		})
		if selection.Else() != nil && len(statements) > 1 {
			builder.SetElse(func() {
				b.visitStatement(statements[1])
			})
		}
		builder.Build()
	case selection.Switch() != nil:
		b.visitSwitchStatement(selection)
	}
}

// switchSegment is one case or default arm of a switch statement.
type switchSegment struct {
	labels    []ssa.Value
	isDefault bool
	body      []cppparser.IStatementContext
}

func (b *singleFileBuilder) visitSwitchStatement(selection *cppparser.SelectionStatementContext) {
	statements := selection.AllStatement()
	if len(statements) == 0 {
		return
	}
	segments := b.splitSwitchSegments(statements[0])

	builder := b.BuildSwitch()
	builder.DirectBodyMerge = true
	builder.BuildCondition(func() ssa.Value {
		return b.visitCondition(selection.Condition())
	})
	var caseSegments []*switchSegment
	var defaultSegment *switchSegment
	for _, segment := range segments {
		if segment.isDefault {
			defaultSegment = segment
			continue
		}
		caseSegments = append(caseSegments, segment)
	}
	builder.BuildCaseSize(len(caseSegments))
	builder.SetCase(func(index int) []ssa.Value {
		if index < len(caseSegments) {
			return caseSegments[index].labels
		}
		return nil
	})
	builder.BuildBody(func(index int) {
		if index < len(caseSegments) {
			for _, statement := range caseSegments[index].body {
				b.visitStatement(statement)
			}
		}
	})
	if defaultSegment != nil {
		builder.BuildDefault(func() {
			for _, statement := range defaultSegment.body {
				b.visitStatement(statement)
			}
		})
	}
	builder.Finish()
}

// splitSwitchSegments groups the statements of a switch body into case arms.
func (b *singleFileBuilder) splitSwitchSegments(raw cppparser.IStatementContext) []*switchSegment {
	var statements []cppparser.IStatementContext
	statement, ok := raw.(*cppparser.StatementContext)
	if ok && statement != nil && statement.CompoundStatement() != nil {
		if block, ok := statement.CompoundStatement().(*cppparser.CompoundStatementContext); ok && block != nil {
			statements = statementListOf(block)
		}
	} else if ok && statement != nil {
		statements = []cppparser.IStatementContext{statement}
	}

	var segments []*switchSegment
	var current *switchSegment
	for _, raw := range statements {
		statement := raw
		// `case 1: case 2: body` nests the labels: the grammar puts the
		// statement after a label inside that labeledStatement. Unwrap every
		// label and keep the innermost statement as body content.
		for statement != nil {
			label := switchLabel(statement)
			if label == nil {
				break
			}
			segment := &switchSegment{}
			isCase := true
			switch {
			case label.ConstantExpression() != nil:
				segment.labels = []ssa.Value{b.evalNode(label.ConstantExpression())}
			case label.Default() != nil:
				segment.isDefault = true
			default:
				// A plain `label:` is not a case arm.
				isCase = false
			}
			if !isCase {
				statement = label.Statement()
				continue
			}
			segments = append(segments, segment)
			current = segment
			statement = label.Statement()
		}
		if statement == nil {
			continue
		}
		if switchLabel(statement) != nil {
			// Only possible when the label is not a case/default.
			continue
		}
		if current == nil {
			current = &switchSegment{}
			segments = append(segments, current)
		}
		current.body = append(current.body, statement)
	}
	return segments
}

func switchLabel(raw cppparser.IStatementContext) *cppparser.LabeledStatementContext {
	statement, ok := raw.(*cppparser.StatementContext)
	if !ok || statement == nil || statement.LabeledStatement() == nil {
		return nil
	}
	labeled, ok := statement.LabeledStatement().(*cppparser.LabeledStatementContext)
	if !ok || labeled == nil {
		return nil
	}
	if labeled.Case() == nil && labeled.Default() == nil {
		return nil
	}
	return labeled
}

func (b *singleFileBuilder) visitIterationStatement(raw cppparser.IIterationStatementContext) {
	iteration, ok := raw.(*cppparser.IterationStatementContext)
	if !ok || iteration == nil {
		return
	}
	switch {
	case iteration.While() != nil:
		condition := iteration.Condition()
		loop := b.CreateLoopBuilder()
		loop.SetCondition(func() ssa.Value {
			return b.visitCondition(condition)
		})
		loop.SetBody(func() {
			if body := iteration.Statement(); body != nil {
				b.visitStatement(body)
			}
		})
		loop.Finish()
	case iteration.Do() != nil:
		// do-while runs the body before the condition. The loop builder always
		// tests first, so the body is emitted once per iteration and exits with
		// a break when the trailing condition is false.
		condition := iteration.Expression()
		loop := b.CreateLoopBuilder()
		loop.SetCondition(func() ssa.Value {
			return b.EmitConstInst(true)
		})
		loop.SetBody(func() {
			if body := iteration.Statement(); body != nil {
				b.visitStatement(body)
			}
			if condition == nil {
				return
			}
			ifBuilder := b.CreateIfBuilder()
			ifBuilder.SetCondition(func() ssa.Value {
				return b.EmitUnOp(ssa.OpNot, b.evalNode(condition))
			}, func() {
				b.Break()
			})
			ifBuilder.Build()
		})
		loop.Finish()
	case iteration.For() != nil:
		b.visitForStatement(iteration)
	}
}

func (b *singleFileBuilder) visitForStatement(iteration *cppparser.IterationStatementContext) {
	statement := iteration.Statement()
	if rangeDeclaration := iteration.ForRangeDeclaration(); rangeDeclaration != nil {
		declaration, ok := rangeDeclaration.(*cppparser.ForRangeDeclarationContext)
		var declaredName string
		var iterable ssa.Value
		if ok && declaration != nil {
			info := b.typeSpecifierInfo(declaration.DeclSpecifierSeq())
			resolved := b.parseDeclarator(declaration.Declarator(), info.typ)
			declaredName = resolved.name
		}
		loop := b.CreateLoopBuilder()
		loop.SetFirst(func() []ssa.Value {
			if initializer := iteration.ForRangeInitializer(); initializer != nil {
				iterable = b.evalNode(initializer)
			}
			return nil
		})
		loop.SetCondition(func() ssa.Value {
			if declaredName != "" && !utils.IsNil(iterable) {
				_, field, nextOK := b.EmitNext(iterable, false)
				if !utils.IsNil(field) {
					b.AssignVariable(b.CreateVariable(declaredName), field)
					if !utils.IsNil(nextOK) {
						return nextOK
					}
				}
			}
			return b.EmitConstInst(true)
		})
		loop.SetBody(func() {
			if statement != nil {
				b.visitStatement(statement)
			}
		})
		loop.Finish()
		return
	}

	init := iteration.ForInitStatement()
	condition := iteration.Condition()
	step := iteration.Expression()

	// Declare the init variables in the enclosing scope before the loop builder
	// snapshots its variables. A variable created inside SetFirst is newer than
	// the loop scope snapshot, so the loop would read its pre-loop constant on
	// every iteration instead of a merged phi.
	b.visitForInitStatement(init)

	loop := b.CreateLoopBuilder()
	loop.SetCondition(func() ssa.Value {
		if condition == nil {
			return b.EmitConstInst(true)
		}
		return b.visitCondition(condition)
	})
	loop.SetBody(func() {
		if statement != nil {
			b.visitStatement(statement)
		}
	})
	loop.SetThird(func() []ssa.Value {
		if step != nil {
			b.evalNode(step)
		}
		return nil
	})
	loop.Finish()
}

// visitForInitStatement lowers the for-init clause in the enclosing block, so
// the declared variables exist before the loop scope is snapshotted.
func (b *singleFileBuilder) visitForInitStatement(raw cppparser.IForInitStatementContext) {
	if raw == nil {
		return
	}
	forInit, ok := raw.(*cppparser.ForInitStatementContext)
	if !ok || forInit == nil {
		return
	}
	if statement, ok := forInit.ExpressionStatement().(*cppparser.ExpressionStatementContext); ok && statement != nil && statement.Expression() != nil {
		b.evalNode(statement.Expression())
		return
	}
	if declaration, ok := forInit.SimpleDeclaration().(*cppparser.SimpleDeclarationContext); ok && declaration != nil {
		b.visitSimpleDeclaration(declaration)
	}
}

func (b *singleFileBuilder) visitJumpStatement(raw cppparser.IJumpStatementContext) {	jump, ok := raw.(*cppparser.JumpStatementContext)
	if !ok || jump == nil {
		return
	}
	switch {
	case jump.Return() != nil:
		var values []ssa.Value
		if expression := jump.Expression(); expression != nil {
			if value := b.evalNode(expression); value != nil {
				values = append(values, value)
			}
		} else if braced := jump.BracedInitList(); braced != nil {
			if value := b.visitBracedInitList(braced, nil); value != nil {
				values = append(values, value)
			}
		}
		b.EmitReturn(values)
	case jump.Break() != nil:
		b.Break()
	case jump.Continue() != nil:
		b.Continue()
	case jump.Goto() != nil:
		// Goto targets are not turned into CFG edges yet; record the call so the
		// statement is not silently dropped.
		// TODO(ssa-cpp): create CFG edges for goto/label instead of a placeholder call.
		target := ""
		if identifier := jump.Identifier(); identifier != nil {
			target = identifier.GetText()
		}
		b.EmitCall(b.NewCall(b.EmitUndefined("goto"), []ssa.Value{b.EmitConstInst(target)}))
	}
}

func (b *singleFileBuilder) visitTryBlock(raw cppparser.ITryBlockContext) {
	tryBlock, ok := raw.(*cppparser.TryBlockContext)
	if !ok || tryBlock == nil {
		return
	}
	builder := b.BuildTry()
	builder.BuildTryBlock(func() {
		b.visitCompoundStatement(tryBlock.CompoundStatement())
	})
	if handlers := tryBlock.HandlerSeq(); handlers != nil {
		seq, ok := handlers.(*cppparser.HandlerSeqContext)
		if ok && seq != nil {
			for _, rawHandler := range seq.AllHandler() {
				handler, ok := rawHandler.(*cppparser.HandlerContext)
				if !ok || handler == nil {
					continue
				}
				name := exceptionParameterName(handler)
				builder.BuildErrorCatch(func() string {
					return name
				}, func() {
					b.visitCompoundStatement(handler.CompoundStatement())
				}, func(value ssa.Value) {
					value.SetType(ssa.CreateErrorType())
				})
			}
		}
	}
	builder.Finish()
}

// exceptionParameterName returns the catch parameter name, or a stable default.
func exceptionParameterName(handler *cppparser.HandlerContext) string {
	if declaration, ok := handler.ExceptionDeclaration().(*cppparser.ExceptionDeclarationContext); ok && declaration != nil {
		if declarator := declaration.Declarator(); declarator != nil {
			if decl, ok := declarator.(*cppparser.DeclaratorContext); ok && decl != nil {
				if id := decl.NoPointerDeclarator(); id != nil {
					if np, ok := id.(*cppparser.NoPointerDeclaratorContext); ok && np != nil && np.DeclaratorId() != nil {
						if name, _ := splitQualifiedName(np.DeclaratorId().GetText()); name != "" {
							return name
						}
					}
				}
			}
		}
	}
	return "exception"
}

func (b *singleFileBuilder) visitCondition(raw cppparser.IConditionContext) ssa.Value {
	condition, ok := raw.(*cppparser.ConditionContext)
	if !ok || condition == nil {
		return b.EmitConstInst(true)
	}
	if expression := condition.Expression(); expression != nil {
		return b.evalNode(expression)
	}
	info := b.typeSpecifierInfo(condition.DeclSpecifierSeq())
	resolved := b.parseDeclarator(condition.Declarator(), info.typ)
	value := b.declareValue(resolved, info)
	if clause := condition.InitializerClause(); clause != nil {
		if assigned := b.evalNode(clause); assigned != nil {
			value = assigned
		}
	} else if braced := condition.BracedInitList(); braced != nil {
		if assigned := b.visitBracedInitList(braced, resolved.typ); assigned != nil {
			value = assigned
		}
	}
	b.AssignVariable(b.CreateVariable(resolved.name), value)
	return value
}

func (b *singleFileBuilder) visitExpressionList(raw cppparser.IExpressionListContext) []ssa.Value {
	list, ok := raw.(*cppparser.ExpressionListContext)
	if !ok || list == nil {
		return nil
	}
	initializerList, ok := list.InitializerList().(*cppparser.InitializerListContext)
	if !ok || initializerList == nil {
		return nil
	}
	return b.visitInitializerList(initializerList)
}

// statementListOf returns the statements of a compound statement.
func statementListOf(block *cppparser.CompoundStatementContext) []cppparser.IStatementContext {
	if block == nil || block.StatementSeq() == nil {
		return nil
	}
	seq, ok := block.StatementSeq().(*cppparser.StatementSeqContext)
	if !ok || seq == nil {
		return nil
	}
	return seq.AllStatement()
}
