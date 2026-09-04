package csharp2ssa

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// 语句编译：块、声明、if/switch、循环、跳转、try、using/lock/yield 等。

const (
	yieldContainerName = "$yield"
	catchExceptionName = "$catchException"
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
	if i.Statement_list() == nil {
		return
	}
	b.BuildSyntaxBlock(func() {
		b.VisitStatementList(i.Statement_list())
	})
}

func (b *singleFileBuilder) VisitStatementList(raw csharpparser.IStatement_listContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Statement_listContext)
	if !ok || i == nil {
		return
	}
	localFunctions := make([]*csharpparser.Local_function_declarationContext, 0)
	for _, statement := range i.AllStatement() {
		if declaration := localFunctionFromStatement(statement); declaration != nil {
			localFunctions = append(localFunctions, declaration)
			b.predeclareLocalFunction(declaration)
		}
	}
	for _, stmt := range i.AllStatement() {
		if b.IsBlockFinish() {
			break
		}
		b.VisitStatement(stmt)
	}
	// Local-function declarations are in scope throughout the containing block,
	// including when their source declaration follows an abrupt statement. Build
	// every remaining shell so calls and analysis can still traverse its body.
	for _, declaration := range localFunctions {
		if shell := b.localFunctionShells[declaration]; shell != nil && !shell.IsFinished() {
			b.VisitLocalFunctionDeclaration(declaration)
		}
	}
}

func localFunctionFromStatement(raw csharpparser.IStatementContext) *csharpparser.Local_function_declarationContext {
	statement, _ := raw.(*csharpparser.StatementContext)
	if statement == nil || statement.Declaration_statement() == nil {
		return nil
	}
	declaration, _ := statement.Declaration_statement().(*csharpparser.Declaration_statementContext)
	if declaration == nil || declaration.Local_function_declaration() == nil {
		return nil
	}
	local, _ := declaration.Local_function_declaration().(*csharpparser.Local_function_declarationContext)
	return local
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
	recoverRange := b.SetRange(raw)
	defer recoverRange()
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
		if cs != nil {
			b.VisitBlock(cs.Block())
		}
	case i.Unchecked_statement() != nil:
		cs, _ := i.Unchecked_statement().(*csharpparser.Unchecked_statementContext)
		if cs != nil {
			b.VisitBlock(cs.Block())
		}
	case i.Unsafe_statement() != nil:
		us, _ := i.Unsafe_statement().(*csharpparser.Unsafe_statementContext)
		if us != nil {
			b.VisitBlock(us.Block())
		}
	case i.Lock_statement() != nil:
		ls, _ := i.Lock_statement().(*csharpparser.Lock_statementContext)
		if ls != nil {
			b.VisitExpression(ls.Expression())
			b.VisitEmbeddedStatement(ls.Embedded_statement())
		}
	case i.Using_statement() != nil:
		b.VisitUsingStatement(i.Using_statement())
	case i.Yield_statement() != nil:
		b.VisitYieldStatement(i.Yield_statement())
	case i.Fixed_statement() != nil:
		fs, _ := i.Fixed_statement().(*csharpparser.Fixed_statementContext)
		if fs != nil {
			b.VisitEmbeddedStatement(fs.Embedded_statement())
		}
	}
}

// ---------------------------------------------------------------- declarations

func (b *singleFileBuilder) VisitDeclarationStatement(raw csharpparser.IDeclaration_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Declaration_statementContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Local_variable_declaration() != nil:
		b.VisitLocalVariableDeclaration(i.Local_variable_declaration())
	case i.Deconstruction_expression() != nil:
		d, _ := i.Deconstruction_expression().(*csharpparser.Deconstruction_expressionContext)
		if d == nil || i.Expression() == nil {
			return
		}
		value := b.VisitExpression(i.Expression())
		if utils.IsNil(value) {
			value = b.EmitUndefined("tuple")
		}
		b.deconstructTuple(d.Deconstruction_tuple(), value)
	case i.Local_constant_declaration() != nil:
		b.VisitLocalConstantDeclaration(i.Local_constant_declaration())
	case i.Local_function_declaration() != nil:
		b.VisitLocalFunctionDeclaration(i.Local_function_declaration())
	case i.Using_declaration() != nil:
		ud, _ := i.Using_declaration().(*csharpparser.Using_declarationContext)
		if ud != nil {
			b.visitNonRefLocalVariableDeclaration(ud.Non_ref_local_variable_declaration())
			b.VisitStatementList(ud.Statement_list())
		}
	}
}

func (b *singleFileBuilder) VisitLocalConstantDeclaration(raw csharpparser.ILocal_constant_declarationContext) {
	i, _ := raw.(*csharpparser.Local_constant_declarationContext)
	if i == nil {
		return
	}
	var typ ssa.Type
	if i.Type_() != nil {
		typ = b.VisitType(i.Type_())
	}
	dc, _ := i.Constant_declarators().(*csharpparser.Constant_declaratorsContext)
	if dc == nil {
		return
	}
	for _, d := range dc.AllConstant_declarator() {
		cd, _ := d.(*csharpparser.Constant_declaratorContext)
		if cd == nil {
			continue
		}
		name := identText(cd.Identifier())
		if name == "" {
			continue
		}
		var value ssa.Value
		if cd.Constant_expression() != nil {
			value = b.VisitExpression(cd.Constant_expression().Expression())
		}
		if utils.IsNil(value) {
			value = b.EmitValueOnlyDeclare(name)
		}
		variable := b.CreateLocalVariable(name)
		b.rememberDeclaredVariableType(variable, typ)
		value = b.applyDeclaredType(value, typ)
		// A declaration must create a local slot even when the initializer is an
		// extern/free value.  CreateVariable would leave the slot non-local and a
		// later assignment with the same source-level name (for example
		// `bool x = a > 0`) is then diagnosed as an extern assignment.
		b.AssignVariable(variable, value)
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
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	typ := b.VisitLocalVariableType(i.Local_variable_type())
	for _, d := range i.AllLocal_variable_declarator() {
		b.visitLocalVariableDeclarator(d, typ)
	}
}

func (b *singleFileBuilder) visitNonRefLocalVariableDeclaration(raw csharpparser.INon_ref_local_variable_declarationContext) {
	i, _ := raw.(*csharpparser.Non_ref_local_variable_declarationContext)
	if i == nil {
		return
	}
	typ := b.VisitLocalVariableType(i.Local_variable_type())
	for _, d := range i.AllLocal_variable_declarator() {
		b.visitLocalVariableDeclarator(d, typ)
	}
}

func (b *singleFileBuilder) visitLocalVariableDeclarator(raw csharpparser.ILocal_variable_declaratorContext, typ ssa.Type) {
	vd, _ := raw.(*csharpparser.Local_variable_declaratorContext)
	if vd == nil {
		return
	}
	name := identText(vd.Identifier())
	if name == "" {
		return
	}
	recoverRange := b.SetRange(vd)
	defer recoverRange()
	var value ssa.Value
	if ic, _ := vd.Local_variable_initializer().(*csharpparser.Local_variable_initializerContext); ic != nil {
		switch {
		case ic.Expression() != nil:
			value = b.VisitExpression(ic.Expression())
		case ic.Array_initializer() != nil:
			value = b.VisitArrayInitializer(ic.Array_initializer(), typ)
		}
	}
	if utils.IsNil(value) {
		value = b.EmitValueOnlyDeclare(name)
	}
	variable := b.CreateLocalVariable(name)
	b.rememberDeclaredVariableType(variable, typ)
	value = b.applyDeclaredType(value, typ)
	b.AssignVariable(variable, value)
}

// ---------------------------------------------------------------- selection

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

func (b *singleFileBuilder) visitBooleanExpression(raw csharpparser.IBoolean_expressionContext) ssa.Value {
	if raw == nil {
		return b.EmitConstInst(true)
	}
	v := b.VisitExpression(raw.Expression())
	if utils.IsNil(v) {
		return b.EmitConstInst(true)
	}
	return v
}

func (b *singleFileBuilder) VisitIfStatement(raw csharpparser.IIf_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.If_statementContext)
	if !ok || i == nil {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	bodies := i.AllEmbedded_statement()
	var conditionBindings []patternBinding
	ifb := b.CreateIfBuilder()
	ifb.AppendItem(
		func() ssa.Value {
			condition := b.visitBooleanExpression(i.Boolean_expression())
			conditionBindings = b.patternBindingsFor(condition)
			return condition
		},
		func() {
			b.bindPatternBindings(conditionBindings)
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

type switchSection struct {
	ctx       *csharpparser.Switch_sectionContext
	patterns  []csharpparser.IPatternContext
	guards    []csharpparser.ICase_guardContext
	isDefault bool
}

func (b *singleFileBuilder) VisitSwitchStatement(raw csharpparser.ISwitch_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Switch_statementContext)
	if !ok || i == nil {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	block, _ := i.Switch_block().(*csharpparser.Switch_blockContext)
	if block == nil {
		return
	}

	var sections []*switchSection
	var defaultSection *switchSection
	allConst := true
	for _, s := range block.AllSwitch_section() {
		sc, _ := s.(*csharpparser.Switch_sectionContext)
		if sc == nil {
			continue
		}
		sec := &switchSection{ctx: sc}
		for _, lab := range sc.AllSwitch_label() {
			lc, _ := lab.(*csharpparser.Switch_labelContext)
			if lc == nil {
				continue
			}
			if lc.DEFAULT() != nil {
				sec.isDefault = true
				continue
			}
			if lc.Case_guard() == nil && isCatchAllPattern(lc.Pattern()) {
				sec.isDefault = true
				if patternDeclaresVariables(lc.Pattern()) {
					sec.patterns = append(sec.patterns, lc.Pattern())
					sec.guards = append(sec.guards, nil)
				}
				continue
			}
			if lc.Case_guard() != nil || !isConstantPattern(lc.Pattern()) {
				allConst = false
			}
			sec.patterns = append(sec.patterns, lc.Pattern())
			sec.guards = append(sec.guards, lc.Case_guard())
		}
		if sec.isDefault {
			if defaultSection == nil {
				defaultSection = sec
			}
			continue
		}
		sections = append(sections, sec)
	}
	sectionBindings := make(map[*switchSection][]patternBinding, len(sections))

	var subject ssa.Value
	sw := b.BuildSwitch()
	sw.AutoBreak = true
	sw.BuildCondition(func() ssa.Value {
		subject = b.visitSelectorExpression(i.Selector_expression())
		if allConst {
			return subject
		}
		return b.EmitConstInst(true)
	})
	sw.BuildCaseSize(len(sections))
	sw.SetCase(func(idx int) []ssa.Value {
		sec := sections[idx]
		var labels []ssa.Value
		for j, p := range sec.patterns {
			if isCatchAllPattern(p) && sec.guards[j] == nil {
				continue
			}
			if allConst {
				labels = append(labels, b.emitPatternConstant(p))
			} else {
				condition, bindings := b.emitArmCondition(subject, p, sec.guards[j])
				sectionBindings[sec] = append(sectionBindings[sec], bindings...)
				labels = append(labels, condition)
			}
		}
		if len(labels) == 0 {
			if allConst {
				// default-only section still needs a case slot; never matches by value
				labels = append(labels, b.EmitUndefined("default"))
			} else {
				labels = append(labels, b.EmitConstInst(false))
			}
		}
		return labels
	})
	body := func(sec *switchSection) {
		bindings := sectionBindings[sec]
		if sec == defaultSection && len(bindings) == 0 {
			for _, p := range sec.patterns {
				bindings = append(bindings, b.catchAllPatternBindings(subject, p)...)
			}
		}
		b.bindPatternBindings(bindings)
		b.VisitStatementList(sec.ctx.Statement_list())
	}
	sw.BuildBody(func(idx int) { body(sections[idx]) })
	if defaultSection != nil {
		sw.BuildDefault(func() { body(defaultSection) })
	}
	b.pushControlTarget(true, false)
	defer b.popControlTarget()
	sw.Finish()
}

func (b *singleFileBuilder) visitSelectorExpression(raw csharpparser.ISelector_expressionContext) ssa.Value {
	sel, _ := raw.(*csharpparser.Selector_expressionContext)
	if sel == nil {
		return b.EmitConstInst(true)
	}
	var v ssa.Value
	if sel.Expression() != nil {
		v = b.VisitExpression(sel.Expression())
	} else if sel.Tuple_expression() != nil {
		v = b.VisitTupleExpression(sel.Tuple_expression())
	}
	if utils.IsNil(v) {
		return b.EmitUndefined("switch")
	}
	return v
}

// ---------------------------------------------------------------- iteration

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
	i, ok := raw.(*csharpparser.While_statementContext)
	if !ok || i == nil {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	loop := b.CreateLoopBuilder()
	loop.SetCondition(func() ssa.Value { return b.visitBooleanExpression(i.Boolean_expression()) })
	loop.SetBody(func() { b.VisitEmbeddedStatement(i.Embedded_statement()) })
	b.pushControlTarget(true, true)
	defer b.popControlTarget()
	loop.Finish()
}

func (b *singleFileBuilder) VisitDoStatement(raw csharpparser.IDo_statementContext) {
	i, ok := raw.(*csharpparser.Do_statementContext)
	if !ok || i == nil {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	conditionName := "$doCondition-" + uuid.NewString()
	first := b.EmitConstInst(true)
	b.AssignVariable(b.CreateLocalVariable(conditionName), first)
	loop := b.CreateLoopBuilder()
	loop.SetCondition(func() ssa.Value { return b.ReadValue(conditionName) })
	loop.SetThird(func() []ssa.Value {
		condition := b.visitBooleanExpression(i.Boolean_expression())
		// The latch updates the header-local slot. Creating another local here
		// would hide it from LoopStmt.Spin and leave the real condition dead.
		b.AssignVariable(b.CreateVariable(conditionName), condition)
		return []ssa.Value{condition}
	})
	loop.SetBody(func() { b.VisitEmbeddedStatement(i.Embedded_statement()) })
	b.pushControlTarget(true, true)
	defer b.popControlTarget()
	loop.Finish()
}

func (b *singleFileBuilder) VisitForStatement(raw csharpparser.IFor_statementContext) {
	i, ok := raw.(*csharpparser.For_statementContext)
	if !ok || i == nil {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	loop := b.CreateLoopBuilder()
	if init, _ := i.For_initializer().(*csharpparser.For_initializerContext); init != nil {
		loop.SetFirst(func() []ssa.Value {
			if init.Local_variable_declaration() != nil {
				b.VisitLocalVariableDeclaration(init.Local_variable_declaration())
				return nil
			}
			return b.VisitStatementExpressionList(init.Statement_expression_list())
		})
	}
	loop.SetCondition(func() ssa.Value {
		fc, _ := i.For_condition().(*csharpparser.For_conditionContext)
		if fc == nil {
			return b.EmitConstInst(true)
		}
		return b.visitBooleanExpression(fc.Boolean_expression())
	})
	if it, _ := i.For_iterator().(*csharpparser.For_iteratorContext); it != nil {
		loop.SetThird(func() []ssa.Value {
			return b.VisitStatementExpressionList(it.Statement_expression_list())
		})
	}
	loop.SetBody(func() { b.VisitEmbeddedStatement(i.Embedded_statement()) })
	b.pushControlTarget(true, true)
	defer b.popControlTarget()
	loop.Finish()
}

func (b *singleFileBuilder) VisitForeachStatement(raw csharpparser.IForeach_statementContext) {
	i, ok := raw.(*csharpparser.Foreach_statementContext)
	if !ok || i == nil {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	loop := b.CreateLoopBuilder()
	var iterable ssa.Value
	loop.SetFirst(func() []ssa.Value {
		iterable = b.VisitExpression(i.Expression())
		if utils.IsNil(iterable) {
			iterable = b.EmitUndefined("foreach")
		}
		return []ssa.Value{iterable}
	})
	loop.SetCondition(func() ssa.Value {
		_, field, okv := b.EmitNext(iterable, false)
		if utils.IsNil(field) {
			field = b.EmitUndefined("item")
		}
		if d, _ := i.Deconstruction_expression().(*csharpparser.Deconstruction_expressionContext); d != nil {
			b.deconstructTuple(d.Deconstruction_tuple(), field)
		} else if name := identText(i.Identifier()); name != "" {
			typ := b.VisitLocalVariableType(i.Local_variable_type())
			variable := b.CreateLocalVariable(name)
			b.rememberDeclaredVariableType(variable, typ)
			field = b.applyDeclaredType(field, typ)
			b.AssignVariable(variable, field)
		}
		if utils.IsNil(okv) {
			okv = b.EmitConstInst(true)
		}
		return okv
	})
	loop.SetBody(func() { b.VisitEmbeddedStatement(i.Embedded_statement()) })
	b.pushControlTarget(true, true)
	defer b.popControlTarget()
	loop.Finish()
}

// ---------------------------------------------------------------- jump

func (b *singleFileBuilder) VisitJumpStatement(raw csharpparser.IJump_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Jump_statementContext)
	if !ok || i == nil {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	switch {
	case i.Return_statement() != nil:
		rs, _ := i.Return_statement().(*csharpparser.Return_statementContext)
		if rs == nil {
			return
		}
		var v ssa.Value
		if rs.Expression() != nil {
			v = b.VisitExpression(rs.Expression())
		} else if rs.Variable_reference() != nil {
			v = b.VisitVariableReference(rs.Variable_reference())
		}
		if utils.IsNil(v) {
			b.emitReturnWithFinalizers(nil)
			return
		}
		b.emitReturnWithFinalizers([]ssa.Value{v})
	case i.Break_statement() != nil:
		b.emitBreakWithFinalizers()
	case i.Continue_statement() != nil:
		b.emitContinueWithFinalizers()
	case i.Throw_statement() != nil:
		ts, _ := i.Throw_statement().(*csharpparser.Throw_statementContext)
		if ts == nil {
			return
		}
		var v ssa.Value
		if ts.Expression() != nil {
			v = b.VisitExpression(ts.Expression())
		}
		if utils.IsNil(v) {
			// rethrow: reuse the innermost catch variable if any
			v = b.PeekValue(catchExceptionName)
			if utils.IsNil(v) {
				v = b.PeekValue("ex")
			}
		}
		if utils.IsNil(v) {
			v = b.EmitUndefined("throw")
		}
		b.EmitPanic(v)
	case i.Goto_statement() != nil:
		gs, _ := i.Goto_statement().(*csharpparser.Goto_statementContext)
		if gs != nil && gs.Constant_expression() != nil {
			b.VisitExpression(gs.Constant_expression().Expression())
		}
	}
}

// emitReturnWithFinalizers emits the abrupt-return edge required by C#:
// evaluate the return expression first (the caller has already done so), then
// run each enclosing finally from inner to outer, and only then emit Return.
//
// A finally may itself return. In that case its return supersedes the pending
// one and is responsible for running any still-enclosing finally clauses. The
// serial avoids relying on FunctionBuilder.IsReturn, which is function-wide
// and may already be true because another CFG branch returned earlier.
func (b *singleFileBuilder) emitReturnWithFinalizers(values []ssa.Value) {
	if b == nil || b.FunctionBuilder == nil {
		return
	}
	if !b.runFinalizersFrom(0) {
		return
	}
	if b.EmitReturn(values) != nil {
		b.returnSerial[b.Function]++
	}
}

// runFinalizersFrom emits the currently-active finally clauses whose lexical
// depth is at or beyond targetDepth.  It returns false when a finalizer itself
// completed the block (return/throw), superseding the pending abrupt transfer.
func (b *singleFileBuilder) runFinalizersFrom(targetDepth int) bool {
	if b == nil || b.FunctionBuilder == nil {
		return false
	}
	function := b.Function
	active := b.activeFinalizers
	if targetDepth < 0 {
		targetDepth = 0
	}
	if targetDepth > len(active) {
		targetDepth = len(active)
	}
	for index := len(active) - 1; index >= targetDepth; index-- {
		finalizer := b.activeFinalizers[index]
		if finalizer == nil || finalizer.function != function || finalizer.body == nil {
			continue
		}
		serial := b.returnSerial[function]
		// The current clause and all clauses lexically inside it must not be
		// re-entered if its body contains another return.
		b.activeFinalizers = active[:index]
		finalizer.body()
		b.activeFinalizers = active
		if b.returnSerial[function] != serial || b.IsBlockFinish() {
			return false
		}
	}
	return true
}

func (b *singleFileBuilder) pushControlTarget(canBreak, canContinue bool) {
	if b == nil {
		return
	}
	b.controlTargets = append(b.controlTargets, csharpControlTarget{
		function:       b.Function,
		finalizerDepth: len(b.activeFinalizers),
		canBreak:       canBreak,
		canContinue:    canContinue,
	})
}

func (b *singleFileBuilder) popControlTarget() {
	if b == nil || len(b.controlTargets) == 0 {
		return
	}
	b.controlTargets = b.controlTargets[:len(b.controlTargets)-1]
}

func (b *singleFileBuilder) abruptTargetDepth(continueTarget bool) (int, bool) {
	for index := len(b.controlTargets) - 1; index >= 0; index-- {
		target := b.controlTargets[index]
		if target.function != b.Function {
			continue
		}
		if (!continueTarget && target.canBreak) || (continueTarget && target.canContinue) {
			return target.finalizerDepth, true
		}
	}
	return 0, false
}

func (b *singleFileBuilder) emitBreakWithFinalizers() {
	depth, ok := b.abruptTargetDepth(false)
	if !ok || !b.runFinalizersFrom(depth) {
		return
	}
	b.Break()
}

func (b *singleFileBuilder) emitContinueWithFinalizers() {
	depth, ok := b.abruptTargetDepth(true)
	if !ok || !b.runFinalizersFrom(depth) {
		return
	}
	b.Continue()
}

// ---------------------------------------------------------------- try

func (b *singleFileBuilder) VisitTryStatement(raw csharpparser.ITry_statementContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Try_statementContext)
	if !ok || i == nil {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	try := b.BuildTry()
	try.BuildTryBlock(func() { b.VisitBlock(i.Block()) })
	if cc, _ := i.Catch_clauses().(*csharpparser.Catch_clausesContext); cc != nil {
		for _, c := range cc.AllSpecific_catch_clause() {
			sc, _ := c.(*csharpparser.Specific_catch_clauseContext)
			if sc == nil {
				continue
			}
			name := catchExceptionName
			var catchType ssa.Type
			if es, _ := sc.Exception_specifier().(*csharpparser.Exception_specifierContext); es != nil {
				// Resolve the type while the enclosing block is still live.  TryBuilder
				// invokes its type callback from the already-finished try entry block.
				if es.Type_() != nil {
					catchType = b.VisitType(es.Type_())
				}
				if n := identText(es.Identifier()); n != "" {
					name = n
				}
			}
			filter, _ := sc.Exception_filter().(*csharpparser.Exception_filterContext)
			try.BuildErrorCatch(func() string { return name }, func() {
				if catchType != nil && catchType.GetTypeKind() != ssa.AnyTypeKind && b.CurrentBlock != nil {
					variable := ssa.GetFristLocalVariableFromScope(b.CurrentBlock.ScopeTable, name)
					b.rememberDeclaredVariableType(variable, catchType)
				}
				if name != catchExceptionName {
					if caught := b.PeekValue(name); !utils.IsNil(caught) {
						b.AssignVariable(b.CreateLocalVariable(catchExceptionName), caught)
					}
				}
				if filter == nil || filter.Boolean_expression() == nil {
					b.VisitBlock(sc.Block())
					return
				}

				condition := b.VisitExpression(filter.Boolean_expression().Expression())
				rethrow := func() {
					caught := b.PeekValue(name)
					if utils.IsNil(caught) {
						caught = b.PeekValue(catchExceptionName)
					}
					if utils.IsNil(caught) {
						caught = b.EmitUndefined(name)
					}
					b.EmitPanic(caught)
				}
				if constant, ok := ssa.ToConstInst(condition); ok && constant.IsBoolean() {
					if constant.Boolean() {
						b.VisitBlock(sc.Block())
					} else {
						rethrow()
					}
					return
				}

				// TryBuilder currently represents lexical catches as parallel
				// ErrorCatch nodes, so its API cannot route a filter-false edge to
				// the next catch. Keep that limitation local: gate this catch body
				// with real CFG and emit a rethrow on the false branch. A future
				// sequential catch-dispatch API can replace only this fallback.
				branch := b.CreateIfBuilder()
				branch.SetCondition(func() ssa.Value { return condition }, func() { b.VisitBlock(sc.Block()) })
				branch.SetElse(rethrow)
				branch.Build()
			}, func(v ssa.Value) {
				if catchType != nil && catchType.GetTypeKind() != ssa.AnyTypeKind {
					v.SetType(catchType)
					return
				}
				v.SetType(ssa.CreateErrorType())
			})
		}
		if g, _ := cc.General_catch_clause().(*csharpparser.General_catch_clauseContext); g != nil {
			try.BuildErrorCatch(func() string { return catchExceptionName }, func() { b.VisitBlock(g.Block()) })
		}
	}
	var finalizer *csharpFinally
	if fc, _ := i.Finally_clause().(*csharpparser.Finally_clauseContext); fc != nil {
		finalizer = &csharpFinally{function: b.Function}
		finalizer.body = func() { b.VisitBlock(fc.Block()) }
		try.BuildFinally(func() {
			// This is TryBuilder's ordinary/error finally block. Keep outer
			// clauses active for a return inside this body, but suppress this
			// clause itself so it cannot recursively inline itself.
			active := b.activeFinalizers
			for index := len(active) - 1; index >= 0; index-- {
				if active[index] == finalizer {
					b.activeFinalizers = append(append([]*csharpFinally(nil), active[:index]...), active[index+1:]...)
					break
				}
			}
			defer func() { b.activeFinalizers = active }()
			finalizer.body()
		})
		b.activeFinalizers = append(b.activeFinalizers, finalizer)
		defer func() { b.activeFinalizers = b.activeFinalizers[:len(b.activeFinalizers)-1] }()
	}
	try.Finish()
}

// ---------------------------------------------------------------- using / yield

func (b *singleFileBuilder) VisitUsingStatement(raw csharpparser.IUsing_statementContext) {
	i, _ := raw.(*csharpparser.Using_statementContext)
	if i == nil {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	b.BuildSyntaxBlock(func() {
		if ra, _ := i.Resource_acquisition().(*csharpparser.Resource_acquisitionContext); ra != nil {
			if ra.Non_ref_local_variable_declaration() != nil {
				b.visitNonRefLocalVariableDeclaration(ra.Non_ref_local_variable_declaration())
			} else if ra.Expression() != nil {
				b.VisitExpression(ra.Expression())
			}
		}
		b.VisitEmbeddedStatement(i.Embedded_statement())
	})
}

// VisitYieldStatement collects `yield return` values into a hidden sequence that the
// enclosing iterator body returns at its end (see finishIteratorBody).
func (b *singleFileBuilder) VisitYieldStatement(raw csharpparser.IYield_statementContext) {
	i, _ := raw.(*csharpparser.Yield_statementContext)
	if i == nil {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	if i.KW_BREAK() != nil {
		if seq := b.PeekValue(yieldContainerName); !utils.IsNil(seq) {
			b.emitReturnWithFinalizers([]ssa.Value{seq})
		} else {
			b.emitReturnWithFinalizers(nil)
		}
		return
	}
	v := b.VisitExpression(i.Expression())
	if utils.IsNil(v) {
		return
	}
	seq := b.PeekValueInThisFunction(yieldContainerName)
	if utils.IsNil(seq) {
		seq = b.EmitMakeBuildWithType(ssa.NewSliceType(ssa.CreateAnyType()), b.EmitConstInst(0), b.EmitConstInst(0))
		b.AssignVariable(b.CreateVariableForce(yieldContainerName), seq)
	}
	idx := len(seq.GetAllMember())
	b.AssignVariable(b.CreateMemberCallVariable(seq, b.EmitConstInst(idx)), v)
}

// visitFunctionBodyBlock compiles a top-level function body block and finalizes iterator bodies.
func (b *singleFileBuilder) visitFunctionBodyBlock(raw csharpparser.IBlockContext) {
	b.VisitBlock(raw)
	b.finishIteratorBody()
}

// finishIteratorBody returns the yield sequence at the end of an iterator body.
func (b *singleFileBuilder) finishIteratorBody() {
	if b.IsBlockFinish() {
		return
	}
	if seq := b.PeekValueInThisFunction(yieldContainerName); !utils.IsNil(seq) {
		b.EmitReturn([]ssa.Value{seq})
	}
}

// ---------------------------------------------------------------- statement expressions

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
		if v := b.VisitStatementExpression(se); !utils.IsNil(v) {
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
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	switch {
	case i.Null_conditional_invocation_expression() != nil:
		return b.VisitNullConditionalInvocationExpression(i.Null_conditional_invocation_expression())
	case i.Primary_expression() != nil:
		return b.VisitPrimaryExpression(i.Primary_expression())
	case i.Assignment() != nil:
		return b.VisitAssignment(i.Assignment())
	case i.Object_creation_expression() != nil:
		return b.VisitObjectCreation(i.Object_creation_expression())
	case i.Post_increment_expression() != nil:
		pi, _ := i.Post_increment_expression().(*csharpparser.Post_increment_expressionContext)
		if pi != nil {
			return b.visitIncDec(pi.Primary_expression(), true, false)
		}
	case i.Post_decrement_expression() != nil:
		pd, _ := i.Post_decrement_expression().(*csharpparser.Post_decrement_expressionContext)
		if pd != nil {
			return b.visitIncDec(pd.Primary_expression(), false, false)
		}
	case i.Pre_increment_expression() != nil:
		pi, _ := i.Pre_increment_expression().(*csharpparser.Pre_increment_expressionContext)
		if pi != nil {
			return b.visitIncDecUnary(pi.Unary_expression(), true, true)
		}
	case i.Pre_decrement_expression() != nil:
		pd, _ := i.Pre_decrement_expression().(*csharpparser.Pre_decrement_expressionContext)
		if pd != nil {
			return b.visitIncDecUnary(pd.Unary_expression(), false, true)
		}
	case i.Await_expression() != nil:
		ae, _ := i.Await_expression().(*csharpparser.Await_expressionContext)
		if ae != nil {
			return b.VisitUnaryExpression(ae.Unary_expression())
		}
	}
	return nil
}
