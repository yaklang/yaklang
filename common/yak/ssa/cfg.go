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
	header := lb.b.NewBasicBlockUnSealed(LoopHeader)
	body := lb.b.NewBasicBlock(LoopBody)
	exit := lb.b.NewBasicBlock(LoopExit)
	latch := lb.b.NewBasicBlock(LoopLatch)
	var loop *Loop
	var init, step []Value
	// build first
	if lb.buildFirst != nil {
		lb.b.CurrentBlock = lb.enter
		init = lb.buildFirst()
	}

	// enter -> header
	lb.b.CurrentBlock = lb.enter
	lb.b.EmitJump(header)

	// build condition
	var condition Value
	// if lb.buildCondition != nil {
	// if in header end; to exit or body
	lb.b.CurrentBlock = header
	condition = lb.buildCondition()
	// } else {
	// 	condition = NewConst(true)
	// lb.b.NewError(Error, SSATAG, "this condition not set!")
	// }
	loop = lb.b.EmitLoop(body, exit, condition)

	// build body
	if lb.buildBody != nil {
		lb.b.CurrentBlock = body
		lb.b.PushTarget(exit, latch, nil)
		lb.buildBody()
		lb.b.PopTarget()
	}

	// body -> latch
	lb.b.EmitJump(latch)

	if len(latch.Preds) != 0 {
		lb.b.CurrentBlock = latch
		// build latch
		if lb.buildThird != nil {
			step = lb.buildThird()
		}
		// latch -> header
		lb.b.EmitJump(header)
	}

	// finish
	header.Sealed()
	loop.Finish(init, step)

	rest := lb.b.NewBasicBlock("")
	lb.b.CurrentBlock = exit
	// exit -> rest
	lb.b.EmitJump(rest)
	lb.b.CurrentBlock = rest
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
	// if instruction
	var doneBlock *BasicBlock
	if i.parent == nil {
		doneBlock = i.b.NewBasicBlock(IfDone)
		i.done = doneBlock
	} else {
		i.done = i.parent.done
		doneBlock = i.parent.done
	}
	trueBlock := i.b.NewBasicBlock(IfTrue)

	// build ifSSA
	cond := i.ifCondition()
	ifSSA := i.b.EmitIf(cond)
	ifSSA.AddTrue(trueBlock)
	// build true block
	i.b.CurrentBlock = trueBlock
	i.ifBody()
	// true -> done
	i.b.EmitJump(doneBlock)

	prevIf := ifSSA
	for index := range i.elifCondition {
		buildCondition := i.elifCondition[index]
		buildBody := i.elifBody[index]
		// set block
		if prevIf.False == nil {
			prevIf.AddFalse(i.b.NewBasicBlock(IfElif))
		}
		i.b.CurrentBlock = prevIf.False
		// build condition
		cond := buildCondition()
		if cond == nil {
			continue
		}
		// build if
		ifSSA := i.b.EmitIf(cond)
		ifSSA.AddTrue(i.b.NewBasicBlock(IfTrue))
		// build if body
		i.b.CurrentBlock = ifSSA.True
		buildBody()
		// if -> done
		i.b.EmitJump(doneBlock)
		prevIf = ifSSA
	}

	if i.elseBody != nil {
		// create false
		prevIf.AddFalse(i.b.NewBasicBlock(IfFalse))
		// build else body
		i.b.CurrentBlock = prevIf.False
		i.elseBody()
		i.b.EmitJump(doneBlock)
	} else if i.child != nil {
		// create elif
		prevIf.AddFalse(i.b.NewBasicBlock(IfElif))
		i.b.CurrentBlock = prevIf.False
		i.child.Finish()
	} else {
		prevIf.AddFalse(doneBlock)
	}

	if i.parent == nil && len(doneBlock.Preds) != 0 {
		i.b.CurrentBlock = doneBlock
		rest := i.b.NewBasicBlock("")
		i.b.EmitJump(rest)
		i.b.CurrentBlock = rest
	}
}

type TryBuilder struct {
	// b
	b *FunctionBuilder

	// block
	enter        *BasicBlock
	buildTry     func()
	buildCatch   func() string
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

func (t *TryBuilder) BuildCatch(f func() string) {
	t.buildCatch = f
}

func (t *TryBuilder) BuildFinally(f func()) {
	t.buildFinally = f
}

func (t *TryBuilder) Finish() {
	var final *BasicBlock
	var id string

	t.b.CurrentBlock = t.enter
	try := t.b.NewBasicBlock(TryStart)
	catch := t.b.NewBasicBlock(TryCatch)
	e := t.b.EmitErrorHandler(try, catch)

	// buildtry
	t.b.CurrentBlock = try
	t.buildTry()

	// buildcatch
	t.b.CurrentBlock = catch
	id = t.buildCatch()
	if id != "" {
		p := NewParam(id, false, t.b.Function)
		p.SetType(BasicTypes[Error])
		t.b.WriteVariable(id, p)
	}

	// buildfinally
	var target *BasicBlock
	if t.buildFinally != nil {
		t.b.CurrentBlock = t.enter
		final = t.b.NewBasicBlock(TryFinally)
		e.AddFinal(final)
		t.b.CurrentBlock = final
		t.buildFinally()

		target = final
	}

	t.b.CurrentBlock = t.enter
	done := t.b.NewBasicBlock("")
	e.AddDone(done)

	if target == nil {
		target = done
	}

	t.b.CurrentBlock = try
	t.b.EmitJump(target)
	t.b.CurrentBlock = catch
	t.b.EmitJump(target)
	if target != done {
		t.b.CurrentBlock = target
		t.b.EmitJump(done)
	}

	t.b.CurrentBlock = done
}

type SwitchBuilder struct {
	// b
	b *FunctionBuilder

	// block
	enter          *BasicBlock
	buildCondition func() Value
	buildHanlder   func() (int, []Value)
	buildBody      func(int)
	buildDefault   func()

	// case
	caseNum int

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

func (t *SwitchBuilder) BuildHanlder(f func() (int, []Value)) {
	t.buildHanlder = f
}

func (t *SwitchBuilder) BuildBody(f func(int)) {
	t.buildBody = f
}

func (t *SwitchBuilder) BuildDefault(f func()) {
	t.buildDefault = f
}

func (t *SwitchBuilder) Finsh() {
	var cond Value
	if t.buildCondition != nil {
		cond = t.buildCondition()
	}

	done := t.b.NewBasicBlock(SwitchDone)
	defaultb := t.b.NewBasicBlock(SwitchDefault)
	t.enter.AddSucc(defaultb)

	// build handler and body
	var exprs []Value
	t.caseNum, exprs = t.buildHanlder()

	handlers := make([]*BasicBlock, 0, t.caseNum)
	slabel := make([]SwitchLabel, 0)
	for i := 0; i < t.caseNum; i++ {
		// build handler
		handler := t.b.NewBasicBlock(SwitchHandler)
		t.enter.AddSucc(handler)
		handlers = append(handlers, handler)
		slabel = append(slabel, NewSwitchLabel(exprs[i], handler))
	}

	NextBlock := func(i int) *BasicBlock {
		if t.DefaultBreak {
			return done
		} else {
			if i == t.caseNum-1 {
				return defaultb
			} else {
				return handlers[i+1]
			}
		}
	}

	for i := 0; i < t.caseNum; i++ {
		// build body
		var _fallthrough *BasicBlock
		if i == t.caseNum-1 {
			_fallthrough = defaultb
		} else {
			_fallthrough = handlers[i+1]
		}
		t.b.PushTarget(done, nil, _fallthrough) // fallthrough just jump to next handler
		// build handlers block
		t.b.CurrentBlock = handlers[i]
		t.buildBody(i)
		// jump handlers-block -> done
		t.b.EmitJump(NextBlock(i))
		t.b.PopTarget()

	}

	// build default
	if t.buildDefault != nil {
		// can't fallthrough
		t.b.PushTarget(done, nil, nil)
		// build default block
		t.b.CurrentBlock = defaultb
		t.buildDefault()
		// jump default -> done
		t.b.EmitJump(done)
		t.b.PopTarget()
	}

	t.b.CurrentBlock = t.enter
	t.b.EmitSwitch(cond, defaultb, slabel)
	rest := t.b.NewBasicBlock("")
	t.b.CurrentBlock = done
	t.b.EmitJump(rest)
	t.b.CurrentBlock = rest
}
