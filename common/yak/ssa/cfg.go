package ssa

import (
	"strings"
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
	return strings.HasPrefix(b.GetVariable(), name)
}

func (b *BasicBlock) GetBlockById(name string) *BasicBlock {
	for _, prev := range b.Preds {
		if prev.IsBlock(name) {
			return prev
		}
	}
	return nil
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
	builder.PushBlockSymbolTable()
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
	builder.PopBlockSymbolTable()
}

// if builder

// enter:
//      // if stmt cond in here
//      If [cond] true -> if.true, false -> if.elif
// if.true: 					<- enter
//      // if-true-body block in here
//      jump if.done
// if.elif: 					<- enter
//      // if-elif cond in here    (this build in "elif" and "else if")
//      If [cond] true -> if.elif_true, false -> if.false
// if.elif_true:				<- if.elif
//      // if-elif-true-body block in here
//      jump if.done
// if.false: 					<- if.elif
//      // if-elif-false-body block in here
//      jump if.done
// if.done:				        <- if.elif_true,if.true,if.false  (target of all if block)
//      jump rest
// rest:
//      ...rest.code....

type IfBuilder struct {
	b *FunctionBuilder
	// enter block
	enter, done *BasicBlock
	// child ifBuilder
	child  *IfBuilder
	parent *IfBuilder

	// if branch
	ifCondition func() Value
	ifBody      func()

	// elif branch
	elifCondition []func() Value
	elifBody      []func()

	// else branch
	elseBody func()
}

func (b *FunctionBuilder) BuildIf() *IfBuilder {
	return &IfBuilder{
		b:             b,
		enter:         b.CurrentBlock,
		elifCondition: make([]func() Value, 0),
		elifBody:      make([]func(), 0),
	}
}

func (i *IfBuilder) BuildChild(child *IfBuilder) {
	i.child = child
	child.parent = i
}

func (i *IfBuilder) BuildCondition(condition func() Value) {
	i.ifCondition = condition
}
func (i *IfBuilder) BuildTrue(body func()) {
	i.ifBody = body
}

func (i *IfBuilder) BuildElif(condition func() Value, body func()) {
	i.elifCondition = append(i.elifCondition, condition)
	i.elifBody = append(i.elifBody, body)
}

func (i *IfBuilder) BuildFalse(body func()) {
	i.elseBody = body
}

func (i *IfBuilder) Finish() {
	builder := i.b
	// if instruction
	var doneBlock *BasicBlock
	// Set end BasicBlock
	if i.parent == nil {
		doneBlock = builder.NewBasicBlockNotAddBlocks(IfDone)
		i.done = doneBlock
	} else {
		i.done = i.parent.done
		doneBlock = i.parent.done
	}
	// TrueBlock
	trueBlock := builder.NewBasicBlockNotAddBlocks(IfTrue)

	// build ifSSA

	// in Enter BasicBlock:
	// enter:
	//      // if stmt cond in here
	// 		// here can be set multiple BasicBlock
	//      If [cond] true -> if.true, false -> if.elif

	// build Condition
	builder.CurrentBlock = i.enter
	// this function can build new cfg
	cond := i.ifCondition()
	// continue append this instruction
	ifSSA := builder.EmitIf(cond)
	ifSSA.AddTrue(trueBlock)

	// build TrueBlock and append this block to Function BasicBlock list.
	// if.true: 					<- enter
	//      // if-true-body block in here
	//      jump if.done
	addToBlocks(trueBlock)
	builder.CurrentBlock = trueBlock
	// this function can build multiple BasicBlock
	i.ifBody()
	builder.EmitJump(doneBlock)

	// if.elif: 					<- enter
	//      // if-elif cond in here    (this build in "elif" and "else if")
	//      If [cond] true -> if.elif_true, false -> if.false
	// if.elif_true:				<- if.elif
	//      // if-elif-true-body block in here
	//      jump if.done
	// if.false: 					<- if.elif
	//      // if-elif-false-body block in here
	//      jump if.done
	prevIf := ifSSA
	for index := range i.elifCondition {
		buildCondition := i.elifCondition[index]
		buildBody := i.elifBody[index]
		// set block
		if prevIf.False == nil {
			prevIf.AddFalse(builder.NewBasicBlock(IfElif))
		}
		builder.CurrentBlock = prevIf.False
		// build condition
		cond := buildCondition()
		if cond == nil {
			continue
		}
		// build if
		ifSSA := builder.EmitIf(cond)
		ifSSA.AddTrue(builder.NewBasicBlock(IfTrue))
		// build if body
		builder.CurrentBlock = ifSSA.True
		buildBody()
		// if -> done
		builder.EmitJump(doneBlock)
		prevIf = ifSSA
	}

	if i.elseBody != nil {
		// if has else stmt, build it and set in False
		// create false
		prevIf.AddFalse(builder.NewBasicBlock(IfFalse))
		// build else body
		builder.CurrentBlock = prevIf.False
		i.elseBody()
		builder.EmitJump(doneBlock)
	} else if i.child != nil {
		// create elif
		prevIf.AddFalse(builder.NewBasicBlock(IfElif))
		// set IfBuilder enter
		i.child.enter = prevIf.False
		i.child.Finish()
	} else {
		prevIf.AddFalse(doneBlock)
	}

	if i.parent == nil && len(doneBlock.Preds) != 0 {
		addToBlocks(doneBlock)
		builder.CurrentBlock = doneBlock
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
	builder.PushBlockSymbolTable()
	builder.CurrentBlock = catch
	id = t.buildError()
	if id != "" {
		p := NewParam(id, false, builder.Function)
		p.SetType(BasicTypes[ErrorType])
		builder.WriteVariable(builder.MapBlockSymbolTable(id), p)
		// builder.WriteVariable(id, p)
	}
	t.buildCatch()
	catch = builder.CurrentBlock
	builder.PopBlockSymbolTable()

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
		builder.PushBlockSymbolTable()
		t.buildBody(i)
		builder.PopBlockSymbolTable()
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
