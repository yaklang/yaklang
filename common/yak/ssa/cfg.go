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
	enter, header, body, exit, latch *BasicBlock

	// b
	b *FunctionBuilder
}

func (b *FunctionBuilder) BuildLoop() *LoopBuilder {
	enter := b.CurrentBlock
	header := b.NewBasicBlockUnSealed(LoopHeader)
	body := b.NewBasicBlock(LoopBody)
	exit := b.NewBasicBlock(LoopExit)
	latch := b.NewBasicBlock(LoopLatch)

	return &LoopBuilder{
		enter:  enter,
		header: header,
		body:   body,
		exit:   exit,
		latch:  latch,
		b:      b,
	}
}

func (lb *LoopBuilder) BuildFirstExpr(f func()) {
	lb.b.CurrentBlock = lb.enter
	f()
}

func (lb *LoopBuilder) BuildCondtion(f func() Value) {
	// enter -> header
	lb.b.CurrentBlock = lb.enter
	lb.b.EmitJump(lb.header)
	// if in header end; to exit or body
	lb.b.CurrentBlock = lb.header
	condition := f()
	ifssa := lb.b.EmitIf(condition)
	ifssa.AddFalse(lb.exit)
	ifssa.AddTrue(lb.body)
}

func (lb *LoopBuilder) BuildLatch(f func()) {
	lb.b.CurrentBlock = lb.latch
	f()
	// latch -> header
	lb.b.EmitJump(lb.header)
}

func (lb *LoopBuilder) BuildBody(f func()) {
	lb.b.CurrentBlock = lb.body
	lb.b.PushTarget(lb.exit, lb.latch, nil)
	f()
	lb.b.PopTarget()
	// body -> latch
	lb.b.EmitJump(lb.latch)
}

func (lb *LoopBuilder) Finish() {
	lb.header.Sealed()
	rest := lb.b.NewBasicBlock("")
	lb.b.CurrentBlock = lb.exit
	// exit -> rest
	lb.b.EmitJump(rest)
	lb.b.CurrentBlock = rest
}

