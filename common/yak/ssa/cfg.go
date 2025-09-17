package ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

const (
	// loop
	LoopHeader    = "loop.header"    // first
	LoopCondition = "loop.condition" // second // condition
	LoopBody      = "loop.body"      // body
	LoopExit      = "loop.exit"      // exit
	LoopLatch     = "loop.latch"     // third // latch

	// if
	IfCondition = "if.condition"
	IfDone      = "if.done"
	IfTrue      = "if.true"
	IfFalse     = "if.false"
	IfElif      = "if.elif"

	// try-catch
	TryStart   = "error.try"
	TryCatch   = "error.catch"
	TryFinally = "error.final"
	TryDone    = "error.done"

	// switch
	SwitchDone    = "switch.done"
	SwitchDefault = "switch.default"
	SwitchHandler = "switch.handler"
	SwitchBlock   = "switch.block"

	// Label jmp
	LabelBlock = "label.block"
	LabelDone  = "label.done"

	// for &&  || ?: expression
	AndExpressionVariable     = "and_expression"
	OrExpressionVariable      = "or_expression"
	TernaryExpressionVariable = "ternary_expression"
)

func (b *BasicBlock) IsBlock(name string) bool {
	return strings.HasPrefix(b.GetName(), name)
}

func (b *BasicBlock) GetBlockById(name string) *BasicBlock {
	for _, id := range b.Preds {
		prev, ok := b.GetValueById(id)
		if !ok || prev == nil {
			continue
		}
		if prev.IsBlock(name) {
			result, ok := ToBasicBlock(prev)
			if !ok {
				log.Warnf("prev(%d): %T is not a *BasicBlock.", prev.GetId(), prev)
				continue
			}
			return result
		}
	}
	return nil
}

// for syntaxBlock

func (b *FunctionBuilder) BuildSyntaxBlock(builder func()) {
	Enter := b.CurrentBlock
	scope := Enter.ScopeTable

	SubBlock := b.NewBasicBlock("")
	b.EmitJump(SubBlock)
	b.CurrentBlock = SubBlock

	endScope := ssautil.BuildSyntaxBlock[Value](ScopeIF(scope), func(svt ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
		b.CurrentBlock.SetScope(svt)
		builder()
		return b.CurrentBlock.ScopeTable
	})

	for _, se := range b.SideEffects {
		if variable := endScope.ReadVariable(se.Name); variable != nil {
			value := variable.GetValue()
			if sideEffect, ok := value.(*SideEffect); ok {
				sideEffect = b.SwitchFreevalueInSideEffect(se.Name, sideEffect, endScope.GetParent())
				variable.Assign(sideEffect)
			}
		}
	}

	if b.CurrentBlock.finish {
		return
	}

	EndBlock := b.NewBasicBlock("")
	EndBlock.SetScope(endScope)

	b.EmitJump(EndBlock)
	b.CurrentBlock = EndBlock
}

// for build loop

// enter:
//
//	       ...
//		    // for first expression in here
//	     jump loop.header
//
// loop.header: 		    <- enter, loop.latch
//
//	// for stmt cond in here
//	If [cond] true -> loop.body, false -> loop.exit
//
// loop.body:	    		<- loop.header
//
//	// for body block in here
//
// loop.latch:              <- loop.body      (target of continue)
//
//	// for third expr in here
//	jump loop.header
//
// loop.exit:	    		<- loop.header    (target of break)
//
//	jump rest
//
// rest:
//
//	...rest.code....
//
// LoopBuilder is a builder for loop statement
type LoopBuilder struct {
	// save data when create
	enter   *BasicBlock
	builder *FunctionBuilder

	condition            func() Value
	body                 func()
	firstExpr, thirdExpr func() []Value
	labelName            string
}

// CreateLoopBuilder Create LoopBuilder
func (b *FunctionBuilder) CreateLoopBuilder() *LoopBuilder {
	return &LoopBuilder{
		enter:     b.CurrentBlock,
		builder:   b,
		labelName: "",
	}
}

func (b *FunctionBuilder) CreateLoopBuilderWithLabelName(labelName string) *LoopBuilder {
	return &LoopBuilder{
		enter:     b.CurrentBlock,
		builder:   b,
		labelName: labelName,
	}
}

// SetFirst : Loop First Expression
func (lb *LoopBuilder) SetFirst(f func() []Value) {
	lb.firstExpr = f
}

// SetCondition : Loop Condition
func (lb *LoopBuilder) SetCondition(f func() Value) {
	lb.condition = f
}

// SetThird : Loop Third Expression
func (lb *LoopBuilder) SetThird(f func() []Value) {
	lb.thirdExpr = f
}

// SetBody : Loop Body
func (lb *LoopBuilder) SetBody(f func()) {
	lb.body = f
}

func (lb *LoopBuilder) Finish() {

	SSABuild := lb.builder
	ExternBlock := SSABuild.CurrentBlock
	scope := ExternBlock.ScopeTable
	header := SSABuild.NewBasicBlock(LoopHeader)
	condition := SSABuild.NewBasicBlockUnSealed(LoopCondition)
	body := SSABuild.NewBasicBlockNotAddBlocks(LoopBody)
	exit := SSABuild.NewBasicBlockNotAddBlocks(LoopExit)
	latchName := ""
	if lb.labelName != "" {
		latchName = fmt.Sprintf("%s-%s", LoopLatch, lb.labelName)
	} else {
		latchName = LoopLatch
	}
	latch := SSABuild.NewBasicBlockNotAddBlocks(latchName)

	LoopBuilder := ssautil.NewLoopStmt(ssautil.ScopedVersionedTableIF[Value](scope), func(name string) Value {
		phi := NewPhi(condition, name)
		condition.Phis = append(condition.Phis, phi.GetId())
		return phi
	})

	LoopBuilder.SetFirst(func(svt ssautil.ScopedVersionedTableIF[Value]) {
		SSABuild.EmitJump(header)
		SSABuild.CurrentBlock = header
		SSABuild.CurrentBlock.SetScope(svt)
		if lb.firstExpr != nil {
			lb.firstExpr()
		}
		SSABuild.EmitJump(condition)
	})

	// var loop *Loop
	LoopBuilder.SetCondition(func(svt ssautil.ScopedVersionedTableIF[Value]) {
		SSABuild.CurrentBlock = condition
		SSABuild.CurrentBlock.SetScope(svt)
		var conditionValue Value
		if lb.condition != nil {
			conditionValue = lb.condition()
		}
		if conditionValue == nil {
			conditionValue = SSABuild.EmitConstInst(true)
		}
		// SSABuild.EmitJump(body)
		SSABuild.EmitLoop(body, exit, conditionValue)
	})

	LoopBuilder.SetBody(func(svt ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
		SSABuild.CurrentBlock = body
		SSABuild.CurrentBlock.SetScope(svt)
		// TODO handle continue and break target

		addToBlocks(body)
		if lb.body != nil {
			SSABuild.PushTarget(LoopBuilder, exit, latch, nil)
			lb.body()
			SSABuild.PopTarget()
		}
		SSABuild.EmitJump(latch)
		return SSABuild.CurrentBlock.ScopeTable
	})
	LoopBuilder.SetThird(func(svt ssautil.ScopedVersionedTableIF[Value]) {
		SSABuild.CurrentBlock = latch
		SSABuild.CurrentBlock.SetScope(svt)
		if lb.thirdExpr != nil {
			lb.thirdExpr()
		}
		SSABuild.EmitJump(condition)
	})
	endScope := LoopBuilder.Build(SpinHandle, generatePhi(SSABuild, latch, lb.enter), generatePhi(SSABuild, exit, lb.enter))

	exit.SetScope(endScope)
	SSABuild.CurrentBlock = exit

	addToBlocks(latch)
	addToBlocks(exit)
}

// if builder

// IfBuilderItem is pair of condition and body, if condition is true, then run body
type IfBuilderItem struct {
	Condition func() Value
	Body      func()
}

// IfBuilder is a builder for if statement
// ssa control flow: if builder
type IfBuilder struct {
	builder *FunctionBuilder
	// enter block
	enter *BasicBlock

	// branch
	items []IfBuilderItem

	elseBody func()
}

// CreateIfBuilder Create IfBuilder
func (b *FunctionBuilder) CreateIfBuilder() *IfBuilder {
	return &IfBuilder{
		builder: b,
		enter:   b.CurrentBlock,
		items:   make([]IfBuilderItem, 0),
	}
}

// AppendItem append IfBuilderItem to IfBuilder
func (i *IfBuilder) AppendItem(cond func() Value, body func()) *IfBuilder {
	i.items = append(i.items, IfBuilderItem{
		Condition: cond,
		Body:      body,
	})
	return i
}

// SetCondition build if condition and body, short for append item
func (i *IfBuilder) SetCondition(cond func() Value, body func()) *IfBuilder {
	i.AppendItem(cond, body)
	return i
}

// SetElse build else  body
func (i *IfBuilder) SetElse(body func()) *IfBuilder {
	i.elseBody = body
	return i
}

// Build if statement
func (i *IfBuilder) Build() *IfBuilder {
	/*
		if-condition :
			condition
			if true -> if-true, false -> if-false
		if-true:
			body
			if-true -> if-done
		if-false:
			// else or else-if
			(else-body)
			(
				condition
				if true -> if-true2, false-> if-false2
			)
			if-false -> if-done
		if-done:
			// next code
	*/
	// just use ssautil scope cfg ScopeBuilder
	SSABuilder := i.builder
	scope := i.enter.ScopeTable
	ScopeBuilder := ssautil.NewIfStmt(ssautil.ScopedVersionedTableIF[Value](scope))

	// if-done block
	DoneBlock := SSABuilder.NewBasicBlockNotAddBlocks(IfDone)
	// DoneBlock.ScopeTable = Scope

	// create if-condition block and jump to it
	conditionBlock := SSABuilder.NewBasicBlock(IfCondition)
	SSABuilder.EmitJump(conditionBlock)
	SSABuilder.CurrentBlock = conditionBlock

	IfStatementBlock := conditionBlock
	createNewIfInst := func(condition Value, trueBlock, falseBlock *BasicBlock) {
		// create if-false block
		// falseBlock := SSABuilder.NewBasicBlock(IfFalse)
		SSABuilder.CurrentBlock = IfStatementBlock
		// create if-instruction in IfStatementBlock
		ifStmt := SSABuilder.EmitIf()
		ifStmt.AddTrue(trueBlock)
		ifStmt.SetCondition(condition)
		ifStmt.AddFalse(falseBlock)

		// currentBlock and  IfStatementBlock is falseBlock
		SSABuilder.CurrentBlock = falseBlock
		IfStatementBlock = falseBlock
	}

	for index, item := range i.items {
		trueBlock := SSABuilder.NewBasicBlock(IfTrue)
		var condition Value
		ScopeBuilder.BuildItem(
			// ifStmt := builder
			func(conditionScope ssautil.ScopedVersionedTableIF[Value]) {
				SSABuilder.CurrentBlock = IfStatementBlock
				SSABuilder.CurrentBlock.SetScope(conditionScope)
				condition = item.Condition()
				IfStatementBlock = SSABuilder.CurrentBlock
			},
			func(bodyScope ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
				SSABuilder.CurrentBlock = trueBlock
				SSABuilder.CurrentBlock.SetScope(bodyScope)
				item.Body()
				if SSABuilder.IsReturn {
					SSABuilder.IsReturn = false
					return SSABuilder.HandlerReturnPhi(bodyScope)
				} else if SSABuilder.CurrentBlock.finish && !SSABuilder.IsReturn {
					return nil
				}
				SSABuilder.EmitJump(DoneBlock)
				return SSABuilder.CurrentBlock.ScopeTable
			},
		)

		if index != len(i.items)-1 {
			falseBlock := SSABuilder.NewBasicBlock(IfFalse)
			createNewIfInst(condition, trueBlock, falseBlock)
		} else {
			// last one
			if i.elseBody != nil {
				// has else
				falseBlock := SSABuilder.NewBasicBlock(IfFalse)
				createNewIfInst(condition, trueBlock, falseBlock)
			} else {
				createNewIfInst(condition, trueBlock, DoneBlock)
			}
		}
	}
	// last one
	if i.elseBody != nil {
		ScopeBuilder.BuildElse(func(sub ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
			SSABuilder.CurrentBlock.SetScope(sub)
			i.elseBody()
			if SSABuilder.CurrentBlock.finish {
				return nil
			}
			return SSABuilder.CurrentBlock.ScopeTable
		})
		SSABuilder.EmitJump(DoneBlock)
	}

	if len(DoneBlock.Preds) != 0 {
		addToBlocks(DoneBlock)
		SSABuilder.CurrentBlock = DoneBlock
		end := ScopeBuilder.BuildFinish(generatePhi(i.builder, DoneBlock, i.enter))
		DoneBlock.SetScope(end)
	}
	return i
}

type tryCatchItem struct {
	exceptionParameter         func() string
	exceptionParameterCallBack func(Value)
	catchBody                  func()
}

type TryBuilder struct {
	// b
	b *FunctionBuilder

	// block
	enter          *BasicBlock
	buildTry       func()
	buildCatchItem []tryCatchItem
	buildFinally   func()
}

func (b *FunctionBuilder) BuildTry() *TryBuilder {
	enter := b.CurrentBlock

	return &TryBuilder{
		enter: enter,
		b:     b,
	}
}

func (t *TryBuilder) BuildTryBlock(f func()) {
	t.buildTry = f
}

func defaultExceptionParameterType(v Value) {
	v.SetType(CreateErrorType())
}

func (t *TryBuilder) BuildErrorCatch(
	err func() string, catch func(),
	callBacks ...func(Value),
) {
	errType := defaultExceptionParameterType
	if len(callBacks) > 0 {
		errType = callBacks[0]
	}

	t.buildCatchItem = append(t.buildCatchItem, tryCatchItem{
		exceptionParameter:         err,
		exceptionParameterCallBack: errType,
		catchBody:                  catch,
	})
}

func (t *TryBuilder) BuildFinally(f func()) {
	t.buildFinally = f
}

func (t *TryBuilder) Finish() {
	builder := t.b
	enter := t.enter
	scope := enter.ScopeTable

	tryBuilder := ssautil.NewTryStmt(ssautil.ScopedVersionedTableIF[Value](scope), generatePhi(builder, nil, t.enter))

	builder.CurrentBlock = t.enter
	tryBlock := builder.NewBasicBlock(TryStart)
	enterTryBlock := tryBlock
	errorHandler := builder.EmitErrorHandler(tryBlock)

	// build try
	tryBuilder.SetTryBody(func(svt ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
		tryBlock.SetScope(svt)
		builder.CurrentBlock = tryBlock
		t.buildTry()
		tryBlock = builder.CurrentBlock
		return tryBlock.ScopeTable
	})

	// build catch
	for _, item := range t.buildCatchItem {
		// catch block
		catchBody := builder.NewBasicBlock(TryCatch)

		// catch exception
		id := item.exceptionParameter()

		builder.CurrentBlock = enterTryBlock
		exception := builder.EmitUndefined(id)
		exception.Kind = UndefinedValueValid
		item.exceptionParameterCallBack(exception)

		// add instruction
		builder.EmitErrorCatch(errorHandler, catchBody, exception)

		// switch to catch bo dy
		builder.CurrentBlock = catchBody
		// add scope and callback
		tryBuilder.AddCache(func(svti ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
			builder.CurrentBlock.SetScope(svti)
			variable := builder.CreateLocalVariable(id)
			builder.AssignVariable(variable, exception)
			// error variable
			// catch body
			if item.catchBody != nil {
				item.catchBody()
			}
			catch := builder.CurrentBlock
			return catch.ScopeTable
		})
	}

	// // build finally
	var target *BasicBlock
	if t.buildFinally != nil {
		final := builder.NewBasicBlock(TryFinally)
		errorHandler.AddFinal(final)
		target = final

		final.SetScope(tryBuilder.CreateFinally())
		builder.CurrentBlock = final
		tryBuilder.SetFinal(func() ssautil.ScopedVersionedTableIF[Value] {
			t.buildFinally()
			final = builder.CurrentBlock
			return final.ScopeTable
		})
	}

	done := builder.NewBasicBlock(TryDone)
	builder.CurrentBlock = done
	end := tryBuilder.Build()
	done.SetScope(end)
	errorHandler.AddDone(done)
	if target == nil {
		target = done
	} else {
		builder.CurrentBlock = target
		builder.EmitJump(done)
	}

	builder.CurrentBlock = tryBlock
	builder.EmitJump(target)
	for _, catchId := range errorHandler.Catch {
		catch, ok := errorHandler.GetValueById(catchId)
		if !ok || catch == nil {
			continue
		}
		builder.CurrentBlock = catch.GetBlock()
		builder.EmitJump(target)
	}

	builder.CurrentBlock = done
}

type SwitchBuilder struct {
	// b
	b *FunctionBuilder

	// block
	enter          *BasicBlock
	buildCondition func() Value
	// TODO: should't use this `func() (int, []Value)`, should have `getCaseSize()int` and `getExpress(int)Value`, just like `buildBody`
	caseSize     int
	buildExpress func(int) []Value
	buildBody    func(int)

	buildDefault func()

	AutoBreak bool
}

func (b *FunctionBuilder) BuildSwitch() *SwitchBuilder {
	enter := b.CurrentBlock

	return &SwitchBuilder{
		b:     b,
		enter: enter,
	}
}

func (t *SwitchBuilder) BuildCondition(f func() Value) {
	t.buildCondition = f
}

func (sw *SwitchBuilder) BuildCaseSize(size int) {
	sw.caseSize = size
}

func (sw *SwitchBuilder) SetCase(f func(int) []Value) {
	sw.buildExpress = f
}

func (t *SwitchBuilder) BuildBody(f func(int)) {
	t.buildBody = f
}

func (t *SwitchBuilder) BuildDefault(f func()) {
	t.buildDefault = f
}

func (t *SwitchBuilder) Finish() {
	builder := t.b
	enter := t.enter
	scope := enter.ScopeTable
	switchBuilder := ssautil.NewSwitchStmt(ssautil.ScopedVersionedTableIF[Value](scope), t.AutoBreak)
	var cond Value

	condb := builder.NewBasicBlockNotAddBlocks("switch-condition")
	done := builder.NewBasicBlockNotAddBlocks(SwitchDone)
	defaultb := builder.NewBasicBlockNotAddBlocks(SwitchDefault)
	builder.EmitJump(condb)
	//t.enter.AddSucc(condb)
	condb.AddSucc(defaultb)

	sLabels := make([]SwitchLabel, 0, t.caseSize)
	handlers := make([]*BasicBlock, 0, t.caseSize)
	blocks := make([]*BasicBlock, 0, t.caseSize)
	for i := 0; i < t.caseSize; i++ {
		handler := builder.NewBasicBlockNotAddBlocks(SwitchHandler)
		block := builder.NewBasicBlockNotAddBlocks(SwitchBlock)
		handlers = append(handlers, handler)
		blocks = append(blocks, block)
	}

	NextBlock := func(i int) *BasicBlock {
		if t.AutoBreak {
			return done
		} else {
			if i == t.caseSize-1 {
				return defaultb
			} else {
				return handlers[i+1]
			}
		}
	}

	if t.buildCondition != nil {
		addToBlocks(condb)
		builder.CurrentBlock = condb

		switchBuilder.BuildCondition(
			func(svt ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
				builder.CurrentBlock.SetScope(svt)
				cond = t.buildCondition()
				return builder.CurrentBlock.ScopeTable
			})
	} else {
		addToBlocks(condb)
		builder.CurrentBlock = condb

		switchBuilder.BuildConditionWithoutExprsion()
	}

	for i := 0; i < t.caseSize; i++ {

		var _fallthrough *BasicBlock
		if i == t.caseSize-1 {
			_fallthrough = defaultb
		} else {
			_fallthrough = handlers[i+1]
		}

		switchBuilder.BuildBody(func(svt ssautil.ScopedVersionedTableIF[Value]) (ssautil.ScopedVersionedTableIF[Value], ssautil.ScopedVersionedTableIF[Value]) {
			builder.CurrentBlock = handlers[i]
			addToBlocks(handlers[i])
			condb.AddSucc(handlers[i])

			builder.CurrentBlock.SetScope(svt)
			vs := t.buildExpress(i)
			builder.EmitJump(blocks[i])
			for _, v := range vs {
				sLabels = append(sLabels, NewSwitchLabel(
					v, handlers[i],
				))
			}
			builder.CurrentBlock = blocks[i]
			addToBlocks(blocks[i])

			body := svt.CreateSubScope()
			builder.CurrentBlock.SetScope(body)
			builder.PushTarget(switchBuilder, done, nil, _fallthrough) // fallthrough just jump to next handler
			t.buildBody(i)
			builder.PopTarget()

			bodyEnd := svt.CreateShadowScope()
			bodyEnd.CoverBy(builder.CurrentBlock.ScopeTable)
			return bodyEnd, svt
		}, generatePhi(builder, handlers[i], condb))

		builder.EmitJump(NextBlock(i))

	}

	// can't fallthrough
	// build default block
	builder.CurrentBlock = defaultb
	// // build default
	addToBlocks(defaultb)
	condb.AddSucc(defaultb)
	switchBuilder.BuildBody(func(svt ssautil.ScopedVersionedTableIF[Value]) (ssautil.ScopedVersionedTableIF[Value], ssautil.ScopedVersionedTableIF[Value]) {
		//builder.CurrentBlock.SetScope(svt)
		body := svt.CreateSubScope()
		builder.CurrentBlock.SetScope(body)
		if t.buildDefault != nil {
			builder.PushTarget(switchBuilder, done, nil, nil)
			t.buildDefault()
			builder.PopTarget()
		}
		bodyEnd := svt.CreateShadowScope()
		bodyEnd.CoverBy(builder.CurrentBlock.ScopeTable)
		return bodyEnd, svt
	}, generatePhi(builder, defaultb, condb))
	// jump default -> done
	builder.EmitJump(done)
	// builder.PopTarget()

	builder.CurrentBlock = condb
	builder.EmitSwitch(cond, defaultb, sLabels)

	if len(done.Preds) == 0 {
		done.finish = true
	} else {
		addToBlocks(done)
	}
	builder.CurrentBlock = done

	end := switchBuilder.Build(generatePhi(builder, done, t.enter))
	done.SetScope(end)
}

type GotoBuilder struct {
	b       *FunctionBuilder
	name    string
	enter   *BasicBlock
	label   *BasicBlock
	isBreak bool
}

func (b *FunctionBuilder) BuildGoto(name string) *GotoBuilder {
	enter := b.CurrentBlock

	return &GotoBuilder{
		b:       b,
		name:    name,
		enter:   enter,
		isBreak: false,
	}
}

func (t *GotoBuilder) SetLabel(label *BasicBlock) {
	t.label = label
}

func (t *GotoBuilder) SetBreak(isBreak bool) {
	t.isBreak = isBreak
}

func (t *GotoBuilder) Finish() func() {
	var _goto *BasicBlock
	var _break *BasicBlock

	builder := t.b
	enter := t.enter.ScopeTable

	if t.isBreak {
		target := builder.target
		/*
			label1:
			for i:=0; i<10; i++ {
				label2:
				for y:=0; y<10; y++ {
					break label1
				}
			}

			LoopStmt ->
				LabelStmt ->
					LoopStmt ->
						LabelStmt ->
		*/
		for ; target.tail != nil; target = target.tail {
			if l, ok := target.tail.LabelTarget.(*ssautil.LabelStmt[Value]); ok {
				if l.GetName() == t.name {
					break
				}
			}
		}
		_break = target._break
		builder.EmitJump(_break)

		return func() { /* 在某些情况下，_break.ScopeTable可能要等loop循环执行完毕后才加载，这里暂时返回一个回调函数 */
			gotoBuilder := ssautil.NewGotoStmt(ssautil.ScopedVersionedTableIF[Value](enter), ssautil.ScopedVersionedTableIF[Value](_break.ScopeTable))
			builder.CurrentBlock.SetScope(gotoBuilder.Build(generatePhi(builder, _break, t.enter)))
		}

	} else {
		_goto = t.label
		builder.EmitJump(_goto)

		return func() {
			gotoBuilder := ssautil.NewGotoStmt(ssautil.ScopedVersionedTableIF[Value](enter), ssautil.ScopedVersionedTableIF[Value](_goto.ScopeTable))
			builder.CurrentBlock.SetScope(gotoBuilder.Build(generatePhi(builder, _goto, t.enter)))
		}
	}
}

type LabelBlockBuilder struct {
	builder   *FunctionBuilder
	enter     *BasicBlock
	labelName string

	labelBlock func()
}

func (b *FunctionBuilder) CreateLabelBlockBuilder(labelName string) *LabelBlockBuilder {
	return &LabelBlockBuilder{
		builder:   b,
		enter:     b.CurrentBlock,
		labelName: labelName,
	}
}

// SetLabelBlock : Label block (LabelBlockBuilder should always have a label block)
func (t *LabelBlockBuilder) SetLabelBlock(f func()) {
	t.labelBlock = f
}

func (t *LabelBlockBuilder) Finish() {
	builder := t.builder
	enterBlock := t.enter
	scope := enterBlock.ScopeTable
	labeledBlock := builder.NewBasicBlockNotAddBlocks(fmt.Sprintf("%s-%s", LabelBlock, t.labelName))
	done := builder.NewBasicBlockNotAddBlocks(fmt.Sprintf("%s-%s", LabelDone, t.labelName))

	labelBlockBuilder := ssautil.NewLabelBlockStmt(ssautil.ScopedVersionedTableIF[Value](scope), t.labelName)

	labelBlockBuilder.SetLabelBlock(func(svt ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
		builder.EmitJump(labeledBlock)
		builder.CurrentBlock = labeledBlock
		builder.CurrentBlock.SetScope(svt)

		addToBlocks(labeledBlock)
		builder.AddLabel(t.labelName, done)
		if t.labelBlock != nil {
			builder.PushTarget(labelBlockBuilder, done, nil, nil)
			t.labelBlock()
			builder.PopTarget()
		}
		return builder.CurrentBlock.ScopeTable
	})

	doneScope := labelBlockBuilder.Build(generatePhi(builder, labeledBlock, enterBlock))

	done.SetScope(doneScope)
	builder.EmitJump(done)
	builder.CurrentBlock = done

	addToBlocks(done)

}

// LabelBuilder is a builder for label statement
type LabelBuilder struct {
	b *FunctionBuilder

	enter        *BasicBlock
	name         string
	gotoHandlers []func(*BasicBlock)
	/* 当某个goto语句遇到一个未解析的label时，可以将goto的finish作为回调函数记录在这里 */
	gotoFinish []func()
}

func (b *FunctionBuilder) BuildLabel(name string) *LabelBuilder {
	enter := b.CurrentBlock

	return &LabelBuilder{
		b:            b,
		enter:        enter,
		name:         name,
		gotoHandlers: []func(*BasicBlock){},
	}
}

func (t *LabelBuilder) SetGotoHandler(f func(*BasicBlock)) {
	t.gotoHandlers = append(t.gotoHandlers, f)
}

func (t *LabelBuilder) GetGotoHandlers() []func(*BasicBlock) {
	return t.gotoHandlers
}

func (t *LabelBuilder) SetGotoFinish(f func()) {
	t.gotoFinish = append(t.gotoFinish, f)
}

func (t *LabelBuilder) GetBlock() *BasicBlock {
	builder := t.b
	block := builder.NewBasicBlockUnSealed(t.name)
	block.SetScope(builder.CurrentBlock.ScopeTable.CreateSubScope())
	return block
}

func (t *LabelBuilder) Build() {
	builder := t.b
	enter := t.enter.ScopeTable
	labelBuilder := ssautil.NewLabelStmt(ssautil.ScopedVersionedTableIF[Value](enter))
	labelBuilder.SetName(t.name)

	target := builder.target
	_break := target._break
	_continue := t.enter

	builder.PushTarget(labelBuilder, _break, _continue, nil)
}

func (t *LabelBuilder) Finish() {
	builder := t.b
	for _, f := range t.gotoFinish {
		f()
	}
	builder.PopTarget()
}

/*
	if(cc){
		a =1
		return
	}

undefind
*/
func (b *FunctionBuilder) HandlerReturnPhi(s ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
	parent := s.GetParent()
	end := parent.CreateSubScope()
	// 更新CurrentBlock.ScopeTable为空scope,避免影响后续PeekValue
	b.CurrentBlock.ScopeTable = end

	names := parent.GetAllVariableNames()
	for name, _ := range names {
		value := b.PeekValue(name)
		if value == nil {
			continue
		}

		if und, ok := ToUndefined(value); ok { // 忽略外部库的function
			if und.Kind == UndefinedValueInValid {
				continue
			}
		}

		if _, ok := ToFunction(value); ok { // 忽略function
			continue
		}
		if _, ok := ToExternLib(value); ok { // 忽略import value
			continue
		}
		if value.GetType().GetTypeKind() == ErrorTypeKind {
			continue
		}

		leftv := b.CreateVariable(name)
		und := b.EmitUndefined(name)
		und.Kind = UndefinedValueReturn
		b.AssignVariable(leftv, und)
	}

	return end
}
