package ssa

import "strings"

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
)

func (b *BasicBlock) IsBlock(name string) bool {
	return strings.HasPrefix(b.Name, name)
}

func (b *BasicBlock) GetBlock(name string) *BasicBlock {
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
//	    // for first expre in here
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

func (lb *LoopBuilder) BuildCondtion(f func() Value) {
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
	if lb.buildCondition != nil {
		// if in header end; to exit or body
		lb.b.CurrentBlock = header
		condition = lb.buildCondition()
	} else {
		condition = NewConst(true)
		lb.b.NewError(Error, SSATAG, "this condition not set!")
	}
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

	// build latch
	if lb.buildThird != nil {
		lb.b.CurrentBlock = latch
		step = lb.buildThird()
	}
	// latch -> header
	lb.b.EmitJump(header)

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

type conditionBuilder func() Value
type IfBuilder struct {
	b *FunctionBuilder
	// enter block
	enter, done *BasicBlock
	// child ifbuilder
	child  *IfBuilder
	parent *IfBuilder

	// if branch
	ifCondition conditionBuilder
	ifBody      func()

	// elif branch
	elifCondition []conditionBuilder
	elifBody      []func()

	// else branch
	elseBody func()
}

func (b *FunctionBuilder) IfBuilder() *IfBuilder {
	return &IfBuilder{
		b:             b,
		enter:         b.CurrentBlock,
		elifCondition: make([]conditionBuilder, 0),
		elifBody:      make([]func(), 0),
	}
}

func (i *IfBuilder) AddChild(child *IfBuilder) {
	i.child = child
	child.parent = i
}

func (i *IfBuilder) IfBranch(condition conditionBuilder, body func()) {
	i.ifCondition = condition
	i.ifBody = body
}

func (i *IfBuilder) ElifBranch(condition conditionBuilder, body func()) {
	i.elifCondition = append(i.elifCondition, condition)
	i.elifBody = append(i.elifBody, body)
}

func (i *IfBuilder) ElseBranch(body func()) {
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

	// build ifssa
	cond := i.ifCondition()
	ifssa := i.b.EmitIf(cond)
	ifssa.AddTrue(trueBlock)
	// build true block
	i.b.CurrentBlock = trueBlock
	i.ifBody()
	// true -> done
	i.b.EmitJump(doneBlock)

	prevIf := ifssa
	for index := range i.elifCondition {
		buildcondition := i.elifCondition[index]
		buildbody := i.elifBody[index]
		// set block
		if prevIf.False == nil {
			prevIf.AddFalse(i.b.NewBasicBlock(IfElif))
		}
		i.b.CurrentBlock = prevIf.False
		// build condition
		cond := buildcondition()
		if cond == nil {
			continue
		}
		// build if
		ifssa := i.b.EmitIf(cond)
		ifssa.AddTrue(i.b.NewBasicBlock(IfTrue))
		// build if body
		i.b.CurrentBlock = ifssa.True
		buildbody()
		// if -> done
		i.b.EmitJump(doneBlock)
		prevIf = ifssa
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

	if i.parent == nil {
		i.b.CurrentBlock = doneBlock
		rest := i.b.NewBasicBlock("")
		i.b.EmitJump(rest)
		i.b.CurrentBlock = rest
	}
}
