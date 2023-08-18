package visitors

import (
	nasl "github.com/yaklang/yaklang/common/yak/antlr4nasl/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm/vmstack"
)

func (c *Compiler) VisitIterationStatement(i nasl.IIterationStatementContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)
	//c.pushScope()
	if forStatement, ok := i.(*nasl.TraditionalForContext); ok {
		c.VisitForStatement(forStatement)
	}
	if whileStatement, ok := i.(*nasl.WhileContext); ok {
		c.VisitWhileStatement(whileStatement)
	}
	if foreachStatement, ok := i.(*nasl.ForEachContext); ok {
		c.VisitForEachStatement(foreachStatement)
	}
	if repeatStatement, ok := i.(*nasl.RepeatContext); ok {
		c.VisitRepeatStatement(repeatStatement)
	}
	//c.pushScopeEnd()
}

func (c *Compiler) VisitForStatement(i *nasl.TraditionalForContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	c.VisitSingleExpression(i.SingleExpression(0))
	ctrlStack := vmstack.New()
	c.TmpData.Push(ctrlStack)
	startP := c.GetCodePostion()
	c.VisitSingleExpression(i.SingleExpression(1))
	jmpEnd := c.pushJmpIfFalse()
	c.VisitStatement(i.Statement())
	startP2 := c.GetCodePostion()
	c.VisitSingleExpression(i.SingleExpression(2))
	jmpStart := c.pushJmp()
	jmpStart.Unary = startP
	endP := c.GetCodePostion()
	jmpEnd.Unary = endP
	for {
		if ctrlStack.Len() == 0 {
			break
		}
		code := ctrlStack.Pop().(*yakvm.Code)
		// 1是break，2是continue
		switch code.Unary {
		case 1:
			code.Unary = endP
		case 2:
			code.Unary = startP2
		}
	}
	c.TmpData.Pop()
}

func (c *Compiler) VisitWhileStatement(i *nasl.WhileContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	ctrlStack := vmstack.New()
	c.TmpData.Push(ctrlStack)
	startP := c.GetCodePostion()
	c.VisitSingleExpression(i.SingleExpression())
	jmpEnd := c.pushJmpIfFalse()
	c.VisitStatement(i.Statement())
	jmpStart := c.pushJmp()
	jmpStart.Unary = startP
	endP := c.GetCodePostion()
	jmpEnd.Unary = endP

	for {
		if ctrlStack.Len() == 0 {
			break
		}
		code := ctrlStack.Pop().(*yakvm.Code)
		// 1是break，2是continue
		switch code.Unary {
		case 1:
			code.Unary = endP
		case 2:
			code.Unary = startP
		}
	}
	c.TmpData.Pop()
}

func (c *Compiler) VisitForEachStatement(i *nasl.ForEachContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	ctrlStack := vmstack.New()
	c.TmpData.Push(ctrlStack)
	//创建迭代器
	iteratorId := c.symbolTable.NewSymbolWithoutName()
	code := c.pushOpcodeFlag(yakvm.OpPushLeftRef)
	code.Unary = iteratorId
	c.pushRef("__NewIterator")
	c.VisitSingleExpression(i.SingleExpression())
	c.pushCall(1)
	c.pushInt(0)
	c.pushBool(false)
	itCode := c.pushOpcodeFlag(yakvm.OpIterableCall)
	itCode.Unary = 1
	c.pushAssigin()
	c.pushOpcodeFlag(yakvm.OpPop)

	// Next()
	itemName := i.Identifier().GetText()
	startP := c.GetCodePostion()
	tmpId := c.symbolTable.NewSymbolWithoutName()
	code = c.pushOpcodeFlag(yakvm.OpPushLeftRef)
	code.Unary = tmpId
	refCode := c.pushOpcodeFlag(yakvm.OpPushRef)
	refCode.Unary = iteratorId
	c.pushString("Next")
	c.pushOpcodeFlag(yakvm.OpMemberCall)
	c.pushCall(0)
	c.pushAssigin()
	c.pushOpcodeFlag(yakvm.OpPop)

	// if !ok {break}
	tmpIdCode := c.pushOpcodeFlag(yakvm.OpPushRef)
	tmpIdCode.Unary = tmpId
	c.pushInt(1)
	c.pushBool(false)
	itCall := c.pushOpcodeFlag(yakvm.OpIterableCall)
	itCall.Unary = 1
	jmpF := c.pushJmpIfFalse()

	//item = iterator.Next()[0]
	c.pushLeftRef(itemName)
	tmpIdCode = c.pushOpcodeFlag(yakvm.OpPushRef)
	tmpIdCode.Unary = tmpId
	c.pushInt(0)
	c.pushBool(false)
	itCall = c.pushOpcodeFlag(yakvm.OpIterableCall)
	itCall.Unary = 1
	c.pushAssigin()

	//for body
	c.VisitStatement(i.Statement())
	jmp := c.pushJmp()
	jmp.Unary = startP
	endP := c.GetCodePostion()
	jmpF.Unary = endP
	for {
		if ctrlStack.Len() == 0 {
			break
		}
		code := ctrlStack.Pop().(*yakvm.Code)
		// 1是break，2是continue
		switch code.Unary {
		case 1:
			code.Unary = endP
		case 2:
			code.Unary = startP
		}
	}
	c.TmpData.Pop()
}

func (c *Compiler) VisitRepeatStatement(i *nasl.RepeatContext) {
	if i == nil {
		return
	}
	c.visitHook(c, i)

	ctrlStack := vmstack.New()
	c.TmpData.Push(ctrlStack)
	startP := c.GetCodePostion()
	c.VisitStatement(i.Statement())
	c.VisitSingleExpression(i.SingleExpression())
	jmpF := c.pushJmpIfFalse()
	jmpF.Unary = startP
	endP := c.GetCodePostion()
	for {
		if ctrlStack.Len() == 0 {
			break
		}
		code := ctrlStack.Pop().(*yakvm.Code)
		// 1是break，2是continue
		switch code.Unary {
		case 1:
			code.Unary = endP
		case 2:
			code.Unary = startP
		}
	}
	c.TmpData.Pop()
}
