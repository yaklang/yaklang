package ssa

import "strings"

const (
	// loop
	LoopHeader = "loop.header"
	LoopBody   = "loop.body"
	LoopExit   = "loop.exit"
	LoopLatch  = "loop.latch"
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

	buildCondition                    func() Value
	buildFirst, buildBody, buildThird func()

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

func (lb *LoopBuilder) BuildFirstExpr(f func()) {
	lb.buildFirst = f
}

func (lb *LoopBuilder) BuildCondtion(f func() Value) {
	lb.buildCondition = f
}

func (lb *LoopBuilder) BuildThird(f func()) {
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
	// build first
	if lb.buildFirst != nil {
		lb.b.CurrentBlock = lb.enter
		lb.buildFirst()
	}

	// build condition
	if lb.buildCondition != nil {
		// enter -> header
		lb.b.CurrentBlock = lb.enter
		lb.b.EmitJump(header)
		// if in header end; to exit or body
		lb.b.CurrentBlock = header
		condition := lb.buildCondition()
		ifssa := lb.b.EmitIf(condition)
		ifssa.AddFalse(exit)
		ifssa.AddTrue(body)
	}

	// build body
	if lb.buildBody != nil {
		lb.b.CurrentBlock = body
		lb.b.PushTarget(exit, latch, nil)
		lb.buildBody()
		lb.b.PopTarget()
		// body -> latch
		lb.b.EmitJump(latch)
	}

	// build latch
	if lb.buildThird != nil {
		lb.b.CurrentBlock = latch
		lb.buildThird()
		// latch -> header
		lb.b.EmitJump(header)
	}

	// finish
	header.Sealed()
	rest := lb.b.NewBasicBlock("")
	lb.b.CurrentBlock = exit
	// exit -> rest
	lb.b.EmitJump(rest)
	lb.b.CurrentBlock = rest
}

