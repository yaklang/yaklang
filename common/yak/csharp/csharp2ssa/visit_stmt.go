package csharp2ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) VisitBlock(raw csharpparser.IBlockContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.BlockContext)
	if !ok || i == nil {
		return
	}
	if i.Statement_list() != nil {
		b.BuildSyntaxBlock(func() {
			b.VisitStatementList(i.Statement_list())
		})
	}
}

func (b *singleFileBuilder) VisitStatementList(raw csharpparser.IStatement_listContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Statement_listContext)
	if !ok || i == nil {
		return
	}
	for _, stmt := range i.AllStatement() {
		if b.IsBlockFinish() {
			return
		}
		b.VisitStatement(stmt)
	}
}

func (b *singleFileBuilder) VisitStatement(raw csharpparser.IStatementContext) {
	if b == nil || raw == nil || b.IsStop() || b.IsBlockFinish() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.StatementContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Embedded_statement() != nil:
		b.VisitEmbeddedStatement(i.Embedded_statement())
	case i.Declaration_statement() != nil:
		b.VisitDeclarationStatement(i.Declaration_statement())
	case i.Labeled_statement() != nil:
		ls, _ := i.Labeled_statement().(*csharpparser.Labeled_statementContext)
		if ls != nil {
			b.VisitStatement(ls.Statement())
		}
	}
}

func (b *singleFileBuilder) VisitEmbeddedStatement(raw csharpparser.IEmbedded_statementContext) {
	if b == nil || raw == nil || b.IsStop() || b.IsBlockFinish() {
		return
	}
	i, ok := raw.(*csharpparser.Embedded_statementContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Block() != nil:
		b.VisitBlock(i.Block())
	case i.Expression_statement() != nil:
		es, _ := i.Expression_statement().(*csharpparser.Expression_statementContext)
		if es != nil {
			b.VisitStatementExpression(es.Statement_expression())
		}
	case i.Selection_statement() != nil:
		b.VisitSelectionStatement(i.Selection_statement())
	case i.Iteration_statement() != nil:
		b.VisitIterationStatement(i.Iteration_statement())
	case i.Jump_statement() != nil:
		b.VisitJumpStatement(i.Jump_statement())
	case i.Try_statement() != nil:
		b.VisitTryStatement(i.Try_statement())
	case i.Checked_statement() != nil:
		cs, _ := i.Checked_statement().(*csharpparser.Checked_statementContext)
		if cs != nil && cs.Block() != nil {
			b.VisitBlock(cs.Block())
		}
	case i.Unchecked_statement() != nil:
		cs, _ := i.Unchecked_statement().(*csharpparser.Unchecked_statementContext)
		if cs != nil && cs.Block() != nil {
			b.VisitBlock(cs.Block())
		}
	case i.Lock_statement() != nil:
		ls, _ := i.Lock_statement().(*csharpparser.Lock_statementContext)
		if ls != nil && ls.Embedded_statement() != nil {
			b.VisitEmbeddedStatement(ls.Embedded_statement())
		}
	case i.Using_statement() != nil:
		us, _ := i.Using_statement().(*csharpparser.Using_statementContext)
		if us != nil && us.Embedded_statement() != nil {
			b.VisitEmbeddedStatement(us.Embedded_statement())
		}
	}
}

func (b *singleFileBuilder) VisitDeclarationStatement(raw csharpparser.IDeclaration_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Declaration_statementContext)
	if !ok || i == nil {
		return
	}
	if i.Local_variable_declaration() != nil {
		b.VisitLocalVariableDeclaration(i.Local_variable_declaration())
		return
	}
	if i.Local_constant_declaration() != nil {
		lc, _ := i.Local_constant_declaration().(*csharpparser.Local_constant_declarationContext)
		if lc != nil && lc.Constant_declarators() != nil {
			dc, _ := lc.Constant_declarators().(*csharpparser.Constant_declaratorsContext)
			if dc != nil {
				for _, d := range dc.AllConstant_declarator() {
					cd, _ := d.(*csharpparser.Constant_declaratorContext)
					if cd == nil {
						continue
					}
					name := identText(cd.Identifier())
					var value ssa.Value
					if cd.Constant_expression() != nil {
						value = b.VisitExpression(cd.Constant_expression().Expression())
					}
					if value == nil {
						value = b.EmitUndefined(name)
					}
					b.AssignVariable(b.CreateVariable(name), value)
				}
			}
		}
	}
}

func (b *singleFileBuilder) VisitLocalVariableDeclaration(raw csharpparser.ILocal_variable_declarationContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Local_variable_declarationContext)
	if !ok || i == nil {
		return
	}
	var typ ssa.Type
	if i.Local_variable_type() != nil && i.Local_variable_type().Type_() != nil {
		typ = b.VisitType(i.Local_variable_type().Type_())
	}
	for _, d := range i.AllLocal_variable_declarator() {
		vd, _ := d.(*csharpparser.Local_variable_declaratorContext)
		if vd == nil {
			continue
		}
		name := identText(vd.Identifier())
		if name == "" {
			continue
		}
		var value ssa.Value
		if init := vd.Local_variable_initializer(); init != nil {
			ic, _ := init.(*csharpparser.Local_variable_initializerContext)
			if ic != nil {
				if ic.Expression() != nil {
					value = b.VisitExpression(ic.Expression())
				} else if ic.Array_initializer() != nil {
					value = b.EmitMakeWithoutType(nil, nil)
				}
			}
		}
		if value == nil {
			value = b.EmitUndefined(name)
		}
		if typ != nil {
			value.SetType(typ)
		}
		b.AssignVariable(b.CreateVariable(name), value)
	}
}

func (b *singleFileBuilder) VisitSelectionStatement(raw csharpparser.ISelection_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Selection_statementContext)
	if !ok || i == nil {
		return
	}
	if i.If_statement() != nil {
		b.VisitIfStatement(i.If_statement())
		return
	}
	if i.Switch_statement() != nil {
		b.VisitSwitchStatement(i.Switch_statement())
	}
}

func (b *singleFileBuilder) VisitIfStatement(raw csharpparser.IIf_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.If_statementContext)
	if !ok || i == nil {
		return
	}
	ifb := b.CreateIfBuilder()
	cond := i.Boolean_expression()
	bodies := i.AllEmbedded_statement()
	ifb.AppendItem(
		func() ssa.Value {
			if cond == nil {
				return b.EmitConstInst(true)
			}
			v := b.VisitExpression(cond.Expression())
			if utils.IsNil(v) {
				return b.EmitConstInst(true)
			}
			return v
		},
		func() {
			if len(bodies) > 0 {
				b.VisitEmbeddedStatement(bodies[0])
			}
		},
	)
	if i.KW_ELSE() != nil && len(bodies) > 1 {
		ifb.SetElse(func() {
			b.VisitEmbeddedStatement(bodies[1])
		})
	}
	ifb.Build()
}

func (b *singleFileBuilder) VisitSwitchStatement(raw csharpparser.ISwitch_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Switch_statementContext)
	if !ok || i == nil || i.Switch_block() == nil {
		return
	}
	block, _ := i.Switch_block().(*csharpparser.Switch_blockContext)
	if block == nil {
		return
	}
	sections := block.AllSwitch_section()
	var defaultIdx = -1
	caseSections := make([]*csharpparser.Switch_sectionContext, 0)
	for _, s := range sections {
		sc, _ := s.(*csharpparser.Switch_sectionContext)
		if sc == nil {
			continue
		}
		isDefault := false
		for _, lab := range sc.AllSwitch_label() {
			lc, _ := lab.(*csharpparser.Switch_labelContext)
			if lc != nil && lc.DEFAULT() != nil {
				isDefault = true
			}
		}
		if isDefault {
			defaultIdx = len(caseSections)
		}
		caseSections = append(caseSections, sc)
	}
	sw := b.BuildSwitch()
	sw.AutoBreak = true
	sw.BuildCondition(func() ssa.Value {
		if i.Selector_expression() == nil {
			return b.EmitConstInst(true)
		}
		sel, _ := i.Selector_expression().(*csharpparser.Selector_expressionContext)
		if sel == nil {
			return b.EmitConstInst(true)
		}
		if sel.Expression() != nil {
			return b.VisitExpression(sel.Expression())
		}
		return b.EmitConstInst(true)
	})
	sw.BuildCaseSize(len(caseSections))
	sw.SetCase(func(idx int) []ssa.Value {
		sc := caseSections[idx]
		var labels []ssa.Value
		for _, lab := range sc.AllSwitch_label() {
			lc, _ := lab.(*csharpparser.Switch_labelContext)
			if lc == nil || lc.DEFAULT() != nil {
				continue
			}
			if lc.Pattern() != nil {
				labels = append(labels, b.VisitPattern(lc.Pattern()))
			}
		}
		if len(labels) == 0 {
			labels = append(labels, b.EmitConstInst(true))
		}
		return labels
	})
	sw.BuildBody(func(idx int) {
		sc := caseSections[idx]
		if sc.Statement_list() != nil {
			b.VisitStatementList(sc.Statement_list())
		}
	})
	if defaultIdx >= 0 {
		idx := defaultIdx
		sw.BuildDefault(func() {
			sc := caseSections[idx]
			if sc.Statement_list() != nil {
				b.VisitStatementList(sc.Statement_list())
			}
		})
	}
	sw.Finish()
}

func (b *singleFileBuilder) VisitIterationStatement(raw csharpparser.IIteration_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Iteration_statementContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.While_statement() != nil:
		b.VisitWhileStatement(i.While_statement())
	case i.Do_statement() != nil:
		b.VisitDoStatement(i.Do_statement())
	case i.For_statement() != nil:
		b.VisitForStatement(i.For_statement())
	case i.Foreach_statement() != nil:
		b.VisitForeachStatement(i.Foreach_statement())
	}
}

func (b *singleFileBuilder) VisitWhileStatement(raw csharpparser.IWhile_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.While_statementContext)
	if !ok || i == nil {
		return
	}
	loop := b.CreateLoopBuilder()
	loop.SetCondition(func() ssa.Value {
		if i.Boolean_expression() == nil {
			return b.EmitConstInst(true)
		}
		v := b.VisitExpression(i.Boolean_expression().Expression())
		if utils.IsNil(v) {
			return b.EmitConstInst(true)
		}
		return v
	})
	loop.SetBody(func() {
		b.VisitEmbeddedStatement(i.Embedded_statement())
	})
	loop.Finish()
}

func (b *singleFileBuilder) VisitDoStatement(raw csharpparser.IDo_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Do_statementContext)
	if !ok || i == nil {
		return
	}
	loop := b.CreateLoopBuilder()
	loop.SetCondition(func() ssa.Value { return b.EmitConstInst(true) })
	if i.Boolean_expression() != nil {
		loop.SetThird(func() []ssa.Value {
			v := b.VisitExpression(i.Boolean_expression().Expression())
			if v == nil {
				return nil
			}
			return []ssa.Value{v}
		})
	}
	loop.SetBody(func() {
		b.VisitEmbeddedStatement(i.Embedded_statement())
	})
	loop.Finish()
}

func (b *singleFileBuilder) VisitForStatement(raw csharpparser.IFor_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.For_statementContext)
	if !ok || i == nil {
		return
	}
	loop := b.CreateLoopBuilder()
	if i.For_initializer() != nil {
		loop.SetFirst(func() []ssa.Value {
			init, _ := i.For_initializer().(*csharpparser.For_initializerContext)
			if init == nil {
				return nil
			}
			if init.Local_variable_declaration() != nil {
				b.VisitLocalVariableDeclaration(init.Local_variable_declaration())
				return nil
			}
			if init.Statement_expression_list() != nil {
				return b.VisitStatementExpressionList(init.Statement_expression_list())
			}
			return nil
		})
	}
	loop.SetCondition(func() ssa.Value {
		if i.For_condition() == nil {
			return b.EmitConstInst(true)
		}
		fc, _ := i.For_condition().(*csharpparser.For_conditionContext)
		if fc == nil || fc.Boolean_expression() == nil {
			return b.EmitConstInst(true)
		}
		v := b.VisitExpression(fc.Boolean_expression().Expression())
		if utils.IsNil(v) {
			return b.EmitConstInst(true)
		}
		return v
	})
	if i.For_iterator() != nil {
		loop.SetThird(func() []ssa.Value {
			it, _ := i.For_iterator().(*csharpparser.For_iteratorContext)
			if it == nil || it.Statement_expression_list() == nil {
				return nil
			}
			return b.VisitStatementExpressionList(it.Statement_expression_list())
		})
	}
	loop.SetBody(func() {
		b.VisitEmbeddedStatement(i.Embedded_statement())
	})
	loop.Finish()
}

func (b *singleFileBuilder) VisitForeachStatement(raw csharpparser.IForeach_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Foreach_statementContext)
	if !ok || i == nil {
		return
	}
	loop := b.CreateLoopBuilder()
	var iterable ssa.Value
	loop.SetFirst(func() []ssa.Value {
		iterable = b.VisitExpression(i.Expression())
		return []ssa.Value{iterable}
	})
	loop.SetCondition(func() ssa.Value {
		name := identText(i.Identifier())
		variable := b.CreateVariable(name)
		_, field, okv := b.EmitNext(iterable, false)
		b.AssignVariable(variable, field)
		if utils.IsNil(okv) {
			okv = b.EmitConstInst(true)
		}
		return okv
	})
	loop.SetBody(func() {
		b.VisitEmbeddedStatement(i.Embedded_statement())
	})
	loop.Finish()
}

func (b *singleFileBuilder) VisitJumpStatement(raw csharpparser.IJump_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Jump_statementContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Return_statement() != nil:
		rs, _ := i.Return_statement().(*csharpparser.Return_statementContext)
		if rs == nil {
			return
		}
		if rs.Expression() != nil {
			v := b.VisitExpression(rs.Expression())
			if v != nil {
				b.EmitReturn([]ssa.Value{v})
				return
			}
		}
		b.EmitReturn(nil)
	case i.Break_statement() != nil:
		b.Break()
	case i.Continue_statement() != nil:
		b.Continue()
	case i.Throw_statement() != nil:
		ts, _ := i.Throw_statement().(*csharpparser.Throw_statementContext)
		if ts != nil && ts.Expression() != nil {
			b.EmitPanic(b.VisitExpression(ts.Expression()))
		}
	}
}

func (b *singleFileBuilder) VisitTryStatement(raw csharpparser.ITry_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Try_statementContext)
	if !ok || i == nil {
		return
	}
	try := b.BuildTry()
	try.BuildTryBlock(func() {
		if i.Block() != nil {
			b.VisitBlock(i.Block())
		}
	})
	if i.Catch_clauses() != nil {
		cc, _ := i.Catch_clauses().(*csharpparser.Catch_clausesContext)
		if cc != nil {
			for _, c := range cc.AllSpecific_catch_clause() {
				sc, _ := c.(*csharpparser.Specific_catch_clauseContext)
				if sc == nil {
					continue
				}
				name := "ex"
				if spec := sc.Exception_specifier(); spec != nil {
					es, _ := spec.(*csharpparser.Exception_specifierContext)
					if es != nil && identText(es.Identifier()) != "" {
						name = identText(es.Identifier())
					}
				}
				catchName := name
				try.BuildErrorCatch(func() string {
					return catchName
				}, func() {
					if sc.Block() != nil {
						b.VisitBlock(sc.Block())
					}
				})
			}
			if gc := cc.General_catch_clause(); gc != nil {
				g, _ := gc.(*csharpparser.General_catch_clauseContext)
				try.BuildErrorCatch(func() string { return "ex" }, func() {
					if g != nil && g.Block() != nil {
						b.VisitBlock(g.Block())
					}
				})
			}
		}
	}
	if i.Finally_clause() != nil {
		fc, _ := i.Finally_clause().(*csharpparser.Finally_clauseContext)
		try.BuildFinally(func() {
			if fc != nil && fc.Block() != nil {
				b.VisitBlock(fc.Block())
			}
		})
	}
	try.Finish()
}

func (b *singleFileBuilder) VisitStatementExpressionList(raw csharpparser.IStatement_expression_listContext) []ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Statement_expression_listContext)
	if !ok || i == nil {
		return nil
	}
	var values []ssa.Value
	for _, se := range i.AllStatement_expression() {
		if v := b.VisitStatementExpression(se); v != nil {
			values = append(values, v)
		}
	}
	return values
}

func (b *singleFileBuilder) VisitStatementExpression(raw csharpparser.IStatement_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Statement_expressionContext)
	if !ok || i == nil {
		return nil
	}
	switch {
	case i.Primary_expression() != nil:
		return b.VisitPrimaryExpression(i.Primary_expression())
	case i.Assignment() != nil:
		return b.VisitAssignment(i.Assignment())
	case i.Object_creation_expression() != nil:
		return b.VisitObjectCreation(i.Object_creation_expression())
	case i.Post_increment_expression() != nil:
		return b.VisitPostIncrement(i.Post_increment_expression())
	case i.Post_decrement_expression() != nil:
		pd, _ := i.Post_decrement_expression().(*csharpparser.Post_decrement_expressionContext)
		if pd == nil {
			return nil
		}
		return b.visitPostfix(pd.Primary_expression(), false)
	case i.Pre_increment_expression() != nil:
		return b.VisitPreIncrement(i.Pre_increment_expression())
	case i.Pre_decrement_expression() != nil:
		pd, _ := i.Pre_decrement_expression().(*csharpparser.Pre_decrement_expressionContext)
		if pd == nil {
			return nil
		}
		return b.visitPrefixUnary(pd.Unary_expression(), false)
	case i.Await_expression() != nil:
		ae, _ := i.Await_expression().(*csharpparser.Await_expressionContext)
		if ae != nil && ae.Unary_expression() != nil {
			return b.VisitUnaryExpression(ae.Unary_expression())
		}
	}
	return nil
}

func (b *singleFileBuilder) VisitPattern(raw csharpparser.IPatternContext) ssa.Value {
	if b == nil || raw == nil {
		return b.EmitConstInst(true)
	}
	i, ok := raw.(*csharpparser.PatternContext)
	if !ok || i == nil {
		return b.EmitConstInst(true)
	}
	if i.Constant_pattern() != nil {
		cp, _ := i.Constant_pattern().(*csharpparser.Constant_patternContext)
		if cp != nil && cp.Constant_expression() != nil {
			return b.VisitExpression(cp.Constant_expression().Expression())
		}
	}
	return b.EmitConstInst(i.GetText())
}
