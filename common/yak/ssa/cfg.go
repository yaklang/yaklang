package ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/log"

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
	IfDone  = "if.done"
	IfTrue  = "if.true"
	IfFalse = "if.false"
	IfElif  = "if.elif"

	// try-catch
	TryStart   = "error.try"
	TryCatch   = "error.catch"
	TryFinally = "error.final"
	TryDone    = "error.done"

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
}

// CreateLoopBuilder Create LoopBuilder
func (b *FunctionBuilder) CreateLoopBuilder() *LoopBuilder {
	return &LoopBuilder{
		enter:   b.CurrentBlock,
		builder: b,
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
	latch := SSABuild.NewBasicBlockNotAddBlocks(LoopLatch)

	LoopBuilder := ssautil.NewLoopStmt(ssautil.ScopedVersionedTableIF[Value](scope), func(name string) Value {
		phi := NewPhi(condition, name)
		condition.Phis = append(condition.Phis, phi)
		return phi
	})

	LoopBuilder.SetFirst(func(svt ssautil.ScopedVersionedTableIF[Value]) {
		SSABuild.CurrentBlock = header
		SSABuild.CurrentBlock.ScopeTable = svt
		if lb.firstExpr != nil {
			lb.firstExpr()
		}
		SSABuild.EmitJump(condition)
	})

	// var loop *Loop
	LoopBuilder.SetCondition(func(svt ssautil.ScopedVersionedTableIF[Value]) {
		SSABuild.CurrentBlock = condition
		SSABuild.CurrentBlock.ScopeTable = svt
		var conditionValue Value
		if lb.condition != nil {
			conditionValue = lb.condition()
		}
		// SSABuild.EmitJump(body)
		SSABuild.EmitLoop(body, exit, conditionValue)
	})

	LoopBuilder.SetBody(func(svt ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
		SSABuild.CurrentBlock = body
		SSABuild.CurrentBlock.ScopeTable = svt
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
		SSABuild.CurrentBlock.ScopeTable = svt
		if lb.thirdExpr != nil {
			lb.thirdExpr()
		}
		SSABuild.EmitJump(condition)
	})
	endScope := LoopBuilder.Build(SpinHandle, generatePhi(SSABuild, latch, lb.enter), generatePhi(SSABuild, exit, lb.enter))

	exit.ScopeTable = endScope
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
	conditionBlock := SSABuilder.NewBasicBlock("if-condition")
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
				SSABuilder.CurrentBlock.ScopeTable = conditionScope
				condition = item.Condition()
				IfStatementBlock = SSABuilder.CurrentBlock
			},
			func(bodyScope ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
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
			SSABuilder.CurrentBlock.ScopeTable = sub
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
		DoneBlock.ScopeTable = end
	}
	return i
}

type tryCatchItem struct {
	err       func() string
	catchBody func()
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

func (t *TryBuilder) BuildErrorCatch(err func() string, catch func()) {
	t.buildCatchItem = append(t.buildCatchItem, tryCatchItem{
		err:       err,
		catchBody: catch,
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
	errorHandler := builder.EmitErrorHandler(tryBlock)

	// build try
	tryBuilder.SetTryBody(func(svt ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
		tryBlock.ScopeTable = svt
		builder.CurrentBlock = tryBlock
		t.buildTry()
		tryBlock = builder.CurrentBlock
		return tryBlock.ScopeTable
	})

	// build catch
	for _, item := range t.buildCatchItem {
		catchBody := builder.NewBasicBlock(TryCatch)
		errorHandler.AddCatch(catchBody)
		builder.CurrentBlock = catchBody
		tryBuilder.AddCache(func(svti ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
			builder.CurrentBlock.ScopeTable = svti
			// error variable
			if id := item.err(); id != "" {
				p := NewParam(id, false, builder)
				p.SetType(BasicTypes[ErrorTypeKind])
				variable := builder.CreateLocalVariable(id)
				builder.AssignVariable(variable, p)
			}
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

		final.ScopeTable = tryBuilder.CreateFinally()
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
	done.ScopeTable = end
	errorHandler.AddDone(done)
	if target == nil {
		target = done
	} else {
		builder.CurrentBlock = target
		builder.EmitJump(done)
	}

	builder.CurrentBlock = tryBlock
	builder.EmitJump(target)
	for _, catch := range errorHandler.catchs {
		builder.CurrentBlock = catch
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
	if t.buildCondition != nil {
		cond = t.buildCondition()
		t.enter = builder.CurrentBlock
	}

	done := builder.NewBasicBlockNotAddBlocks(SwitchDone)
	defaultb := builder.NewBasicBlockNotAddBlocks(SwitchDefault)
	t.enter.AddSucc(defaultb)

	sLabels := make([]SwitchLabel, 0, t.caseSize)
	handlers := make([]*BasicBlock, 0, t.caseSize)
	for i := 0; i < t.caseSize; i++ {
		vs := t.buildExpress(i)
		handler := builder.NewBasicBlockNotAddBlocks(SwitchHandler)
		handlers = append(handlers, handler)

		for _, v := range vs {
			sLabels = append(sLabels, NewSwitchLabel(
				v, handler,
			))
		}
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

	for i := 0; i < t.caseSize; i++ {

		var _fallthrough *BasicBlock
		if i == t.caseSize-1 {
			_fallthrough = defaultb
		} else {
			_fallthrough = handlers[i+1]
		}

		builder.CurrentBlock = handlers[i]

		addToBlocks(handlers[i])
		t.enter.AddSucc(handlers[i])
		switchBuilder.BuildBody(func(svt ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
			builder.CurrentBlock.ScopeTable = svt

			builder.PushTarget(switchBuilder, done, nil, _fallthrough) // fallthrough just jump to next handler
			t.buildBody(i)
			builder.PopTarget()

			return builder.CurrentBlock.ScopeTable
		}, generatePhi(builder, handlers[i], t.enter))

		builder.EmitJump(NextBlock(i))

	}

	// can't fallthrough
	// build default block
	builder.CurrentBlock = defaultb
	// // build default
	addToBlocks(defaultb)
	t.enter.AddSucc(defaultb)
	switchBuilder.BuildBody(func(svt ssautil.ScopedVersionedTableIF[Value]) ssautil.ScopedVersionedTableIF[Value] {
		builder.CurrentBlock.ScopeTable = svt
		if t.buildDefault != nil {
			builder.PushTarget(switchBuilder, done, nil, nil)
			t.buildDefault()
			builder.PopTarget()
		}
		return builder.CurrentBlock.ScopeTable
	}, generatePhi(builder, defaultb, t.enter))
	// jump default -> done
	builder.EmitJump(done)
	// builder.PopTarget()

	builder.CurrentBlock = t.enter
	builder.EmitSwitch(cond, defaultb, sLabels)
	addToBlocks(done)
	builder.CurrentBlock = done
	end := switchBuilder.Build(generatePhi(builder, done, t.enter))
	done.ScopeTable = end
}
