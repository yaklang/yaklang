package ssa

import (
	"strings"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

const (
	// loop
	LoopHeader = "loop.header"
	LoopBody   = "loop.body"
	LoopExit   = "loop.exit"
	LoopLatch  = "loop.latch"

	// if
	IfDone  = "if.done"
	IfTrue  = "if.true"
	IfFalse = "if.false"
	IfElif  = "if.elif"

	// try-catch
	TryStart   = "error.try"
	TryCatch   = "error.catch"
	TryFinally = "error.final"
	TryDone    = ""

	// switch
	SwitchDone    = "switch.done"
	SwitchDefault = "switch.default"
	SwitchHandler = "switch.handler"
)

func (b *BasicBlock) IsBlock(name string) bool {
	return strings.HasPrefix(b.GetName(), name)
}

func (b *BasicBlock) GetBlockById(name string) *BasicBlock {
	for _, prev := range b.Preds {
		if prev.IsBlock(name) {
			return prev
		}
	}
	return nil
}

// for syntaxBlock

func (b *FunctionBuilder) BuildSyntaxBlock(builder func()) {
	Enter := b.CurrentBlock
	Scope := Enter.ScopeTable

	SubBlock := b.NewBasicBlock("")
	b.EmitJump(SubBlock)
	b.CurrentBlock = SubBlock

	endScope := ssautil.BuildSyntaxBlock(Scope, func(svt *ssautil.ScopedVersionedTable[*Variable]) *ssautil.ScopedVersionedTable[*Variable] {
		b.CurrentBlock.ScopeTable = svt
		builder()
		return b.CurrentBlock.ScopeTable
	})

	if b.CurrentBlock.finish {
		return
	}

	EndBlock := b.NewBasicBlock("")
	EndBlock.ScopeTable = endScope

	b.EmitJump(EndBlock)
	b.CurrentBlock = EndBlock
}

// for build loop

// enter:
//        ...
//	    // for first expression in here
//      jump loop.header
// loop.header: 		    <- enter, loop.latch
//      // for stmt cond in here
//      If [cond] true -> loop.body, false -> loop.exit
// loop.body:	    		<- loop.header
//      // for body block in here
// loop.latch:              <- loop.body      (target of continue)
//      // for third expr in here
//      jump loop.header
// loop.exit:	    		<- loop.header    (target of break)
//      jump rest
// rest:
//      ...rest.code....

type LoopBuilder struct {
	// block
	enter *BasicBlock

	buildCondition         func() Value
	buildBody              func()
	buildFirst, buildThird func() []Value

	// b
	b *FunctionBuilder
}

func (b *FunctionBuilder) BuildLoop() *LoopBuilder {
	enter := b.CurrentBlock

	return &LoopBuilder{
		enter: enter,
		b:     b,
	}
}

func (lb *LoopBuilder) BuildFirstExpr(f func() []Value) {
	lb.buildFirst = f
}

func (lb *LoopBuilder) BuildCondition(f func() Value) {
	lb.buildCondition = f
}

func (lb *LoopBuilder) BuildThird(f func() []Value) {
	lb.buildThird = f
}

func (lb *LoopBuilder) BuildBody(f func()) {
	lb.buildBody = f
}

func (lb *LoopBuilder) Finish() {
	builder := lb.b
	header := builder.NewBasicBlockUnSealed(LoopHeader)
	body := builder.NewBasicBlockNotAddBlocks(LoopBody)
	exit := builder.NewBasicBlockNotAddBlocks(LoopExit)
	latch := builder.NewBasicBlockNotAddBlocks(LoopLatch)
	// loop is a scope
	// builder.ScopeStart()
	var loop *Loop
	var init, step []Value
	// build first
	if lb.buildFirst != nil {
		builder.CurrentBlock = lb.enter
		init = lb.buildFirst()
		lb.enter = builder.CurrentBlock
	}

	// enter -> header
	builder.CurrentBlock = lb.enter
	builder.EmitJump(header)

	// build condition
	var condition Value
	builder.CurrentBlock = header
	condition = lb.buildCondition()
	loop = builder.EmitLoop(body, exit, condition)

	// build body
	if lb.buildBody != nil {
		addToBlocks(body)
		builder.CurrentBlock = body
		builder.PushTarget(exit, latch, nil)
		lb.buildBody()
		builder.PopTarget()
	}

	// body -> latch
	builder.EmitJump(latch)

	if len(latch.Preds) != 0 {
		builder.CurrentBlock = latch
		// build latch
		if lb.buildThird != nil {
			step = lb.buildThird()
		}
		// latch -> header
		builder.EmitJump(header)
	}

	// finish
	header.Sealed()
	loop.Finish(init, step)

	addToBlocks(latch)
	addToBlocks(exit)
	// rest := builder.NewBasicBlock("")
	builder.CurrentBlock = exit
	// // exit -> rest
	// builder.EmitJump(rest)
	// builder.CurrentBlock = rest
	// builder.ScopeEnd()
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
func (i *IfBuilder) AppendItem(item IfBuilderItem) *IfBuilder {
	i.items = append(i.items, item)
	return i
}

// SetElse build else  body
func (i *IfBuilder) SetElse(body func()) *IfBuilder {
	i.elseBody = body
	return i
}

// build phi
func generalPhi(builder *FunctionBuilder) func(name string, t []*Variable) *Variable {
	scope := builder.CurrentBlock.ScopeTable
	return func(name string, t []*Variable) *Variable {
		values := lo.Map(t, func(v *Variable, _ int) Value { return v.value })
		// log.Infof("generalPhi: %s %v", name, values)
		phi := builder.EmitPhi(name, values)
		if phi == nil {
			return nil
		}
		v := NewVariable(name, builder.CurrentRange, scope)
		builder.AssignVariable(v, phi)
		return v
	}
}

// Build if statement
func (i *IfBuilder) Build() {
	// just use ssautil scope cfg ScopeBuilder
	SSABuilder := i.builder
	Scope := i.enter.ScopeTable
	ScopeBuilder := ssautil.NewIfStmt(Scope)
	// done block
	DoneBlock := SSABuilder.NewBasicBlockNotAddBlocks(IfDone)
	// DoneBlock.ScopeTable = Scope

	conditionBlock := SSABuilder.NewBasicBlock("if-condition")
	SSABuilder.EmitJump(conditionBlock)
	SSABuilder.CurrentBlock = conditionBlock

	CurrentBlock := conditionBlock
	for _, item := range i.items {
		trueBlock := SSABuilder.NewBasicBlock(IfTrue)
		var condition Value
		ScopeBuilder.BuildItem(
			// ifStmt := builder
			func(conditionScope *ssautil.ScopedVersionedTable[*Variable]) {
				SSABuilder.CurrentBlock = CurrentBlock
				SSABuilder.CurrentBlock.ScopeTable = conditionScope
				condition = item.Condition()
			},
			func(bodyScope *ssautil.ScopedVersionedTable[*Variable]) *ssautil.ScopedVersionedTable[*Variable] {
				SSABuilder.CurrentBlock = trueBlock
				SSABuilder.CurrentBlock.ScopeTable = bodyScope
				item.Body()
				if SSABuilder.CurrentBlock.finish {
					return nil
				}
				SSABuilder.EmitJump(DoneBlock)
				return SSABuilder.CurrentBlock.ScopeTable
			},
		)
		falseBlock := SSABuilder.NewBasicBlock(IfFalse)
		SSABuilder.CurrentBlock = CurrentBlock
		ifStmt := SSABuilder.EmitIf()
		ifStmt.AddTrue(trueBlock)
		ifStmt.SetCondition(condition)
		ifStmt.AddFalse(falseBlock)

		SSABuilder.CurrentBlock = falseBlock
		CurrentBlock = falseBlock
	}

	if i.elseBody != nil {
		ScopeBuilder.BuildElse(func(sub *ssautil.ScopedVersionedTable[*Variable]) *ssautil.ScopedVersionedTable[*Variable] {
			SSABuilder.CurrentBlock.ScopeTable = sub
			i.elseBody()
			if SSABuilder.CurrentBlock.finish {
				return nil
			}
			return SSABuilder.CurrentBlock.ScopeTable
		})
	}
	SSABuilder.EmitJump(DoneBlock)

	if len(DoneBlock.Preds) != 0 {
		addToBlocks(DoneBlock)
		SSABuilder.CurrentBlock = DoneBlock
		end := ScopeBuilder.BuildFinish(generalPhi(i.builder))
		DoneBlock.ScopeTable = end
	}
}

type TryBuilder struct {
	// b
	b *FunctionBuilder

	// block
	enter        *BasicBlock
	buildTry     func()
	buildError   func() string
	buildCatch   func()
	buildFinally func()
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

func (t *TryBuilder) BuildError(f func() string) {
	t.buildError = f
}

func (t *TryBuilder) BuildCatch(f func()) {
	t.buildCatch = f
}

func (t *TryBuilder) BuildFinally(f func()) {
	t.buildFinally = f
}

func (t *TryBuilder) Finish() {
	var final *BasicBlock
	var id string
	builder := t.b

	builder.CurrentBlock = t.enter
	try := builder.NewBasicBlock(TryStart)
	catch := builder.NewBasicBlock(TryCatch)
	e := builder.EmitErrorHandler(try, catch)

	// build try
	builder.CurrentBlock = try
	t.buildTry()
	try = builder.CurrentBlock

	// build catch
	// builder.ScopeStart()
	builder.CurrentBlock = catch
	id = t.buildError()
	if id != "" {
		p := NewParam(id, false, builder)
		p.SetType(BasicTypes[ErrorType])
		// builder.WriteVariable(builder.SetScopeLocalVariable(id), p)
		// builder.WriteVariable(id, p)
	}
	t.buildCatch()
	catch = builder.CurrentBlock
	// builder.ScopeEnd()

	// build finally
	var target *BasicBlock
	if t.buildFinally != nil {
		builder.CurrentBlock = t.enter
		final = builder.NewBasicBlock(TryFinally)
		e.AddFinal(final)
		target = final
	}

	builder.CurrentBlock = t.enter
	done := builder.NewBasicBlock("")
	e.AddDone(done)

	if target == nil {
		target = done
	}

	builder.CurrentBlock = try
	builder.EmitJump(target)
	builder.CurrentBlock = catch
	builder.EmitJump(target)

	if t.buildFinally != nil {
		// if target != done {
		builder.CurrentBlock = final
		t.buildFinally()
		builder.EmitJump(done)
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
	buildHandler func() (int, []Value)
	buildBody    func(int)
	buildDefault func()

	DefaultBreak bool
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

func (t *SwitchBuilder) BuildHandler(f func() (int, []Value)) {
	t.buildHandler = f
}

func (t *SwitchBuilder) BuildBody(f func(int)) {
	t.buildBody = f
}

func (t *SwitchBuilder) BuildDefault(f func()) {
	t.buildDefault = f
}

func (t *SwitchBuilder) Finish() {
	builder := t.b
	var cond Value
	if t.buildCondition != nil {
		cond = t.buildCondition()
		t.enter = builder.CurrentBlock
	}

	done := builder.NewBasicBlockNotAddBlocks(SwitchDone)
	defaultb := builder.NewBasicBlockNotAddBlocks(SwitchDefault)
	t.enter.AddSucc(defaultb)

	// build handler and body
	var exprs []Value
	caseNum, exprs := t.buildHandler()

	handlers := make([]*BasicBlock, 0, caseNum)
	slabel := make([]SwitchLabel, 0, caseNum)
	for i := 0; i < caseNum; i++ {
		// build handler
		handler := builder.NewBasicBlock(SwitchHandler)
		t.enter.AddSucc(handler)
		handlers = append(handlers, handler)
		slabel = append(slabel, NewSwitchLabel(exprs[i], handler))
	}

	NextBlock := func(i int) *BasicBlock {
		if t.DefaultBreak {
			return done
		} else {
			if i == caseNum-1 {
				return defaultb
			} else {
				return handlers[i+1]
			}
		}
	}

	for i := 0; i < caseNum; i++ {
		// build body
		var _fallthrough *BasicBlock
		if i == caseNum-1 {
			_fallthrough = defaultb
		} else {
			_fallthrough = handlers[i+1]
		}
		builder.PushTarget(done, nil, _fallthrough) // fallthrough just jump to next handler
		// build handlers block
		builder.CurrentBlock = handlers[i]
		// builder.ScopeStart()
		t.buildBody(i)
		// builder.ScopeEnd()
		// jump handlers-block -> done
		builder.EmitJump(NextBlock(i))
		builder.PopTarget()
	}

	// can't fallthrough
	builder.PushTarget(done, nil, nil)
	// build default block
	builder.CurrentBlock = defaultb
	// build default
	if t.buildDefault != nil {
		t.buildDefault()
	}
	// jump default -> done
	builder.EmitJump(done)
	builder.PopTarget()

	builder.CurrentBlock = t.enter
	builder.EmitSwitch(cond, defaultb, slabel)
	addToBlocks(done)
	addToBlocks(defaultb)
	builder.CurrentBlock = done
}
