package luaast

import (
	"fmt"
	uuid "github.com/satori/go.uuid"
	"yaklang/common/log"
	lua "yaklang/common/yak/antlr4Lua/parser"
	"yaklang/common/yak/antlr4yak/yakvm"
	"strings"
)

func (l *LuaTranslator) VisitStat(raw lua.IStatContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.StatContext)

	if i == nil {
		return nil
	}

	if s := i.SemiColon(); s != nil {
		return nil
	}

	if s := i.Varlist(); s != nil {
		if t := i.Explist(); t != nil {
			l.VisitExpList(t)
		}
		l.VisitVarList(true, s)
		l.pushGlobalAssign()
		return nil
	}

	if s := i.Functioncall(); s != nil {
		l.VisitFunctionCall(s)
		l.pushOpPop()
		return nil
	}

	if s := i.Label(); s != nil {
		l.VisitLabel(s)
		return nil
	}

	// The break statement terminates the execution of a `while`, `repeat`, or `for` loop, skipping to the next statement after the loop
	if s := i.Break(); s != nil {
		if !(l.NowInWhile() || l.NowInRepeat() || l.NowInFor()) {
			log.Warnf("Syntax Error: Break should be in `while`, `repeat`, or `for` loop, it will crash in future")
		}

		l.pushBreak()
		return nil
	}

	// The goto statement transfers the program control to a label. For syntactical reasons, labels in Lua are considered statements too
	if s := i.Goto(); s != nil {
		l.VisitGoto(i.NAME().GetText())
		return nil
	}

	if s, w, f := i.Do(), i.While(), i.For(); s != nil && w == nil && f == nil {
		l.VisitBlock(i.Block(0))
		return nil
	}

	// no `continue` in lua
	if s := i.While(); s != nil {
		/*
			while-scope
			exp
			jmp-to-end-if-false
			block-scope
			block-scope-end
			jmp-to-exp
			while-scope-end

			当存在break时
			while-scope
			exp
			jmp-to-end-if-false
			block-scope
			break
			block-scope-end
			jmp-to-exp
			while-scope-end
		*/
		var toEnds []*yakvm.Code

		startIndex := l.GetNextCodeIndex()
		l.enterWhileContext(startIndex)

		f := l.SwitchSymbolTableInNewScope("while-loop", uuid.NewV4().String())

		exp := i.Exp(0)
		l.VisitExp(exp)
		toEnds = append(toEnds, l.pushJmpIfFalse())

		if i.Do() != nil {
			l.VisitBlock(i.Block(0))
		}

		l.pushJmp().Unary = startIndex + 1 // avoid jmp to new-scope
		var whileEnd = l.GetNextCodeIndex()

		f()
		// 设置解析的 block 中没有设置过的 break
		l.exitWhileContext(whileEnd + 1)

		// 设置条件自带的 toEnd 位置
		for _, toEnd := range toEnds {
			if toEnd != nil {
				toEnd.Unary = whileEnd
			}
		}

		return nil
	}
	//In the repeat–until loop, the inner block does not end at the until keyword,
	//but only after the condition. So, the condition can refer to local variables
	//declared inside the loop block
	// 考虑用语法糖的形式实现
	if s := i.Repeat(); s != nil {
		var toEnds []*yakvm.Code

		startIndex := l.GetNextCodeIndex()
		l.enterWhileContext(startIndex)

		f := l.SwitchSymbolTableInNewScope("repeat-loop", uuid.NewV4().String())

		l.VisitBlock(i.Block(0))
		// 把exp条件放进block-scope 这样在block里生命的local在until进行条件判断时仍可使用
		var endScope *yakvm.Code
		endScope, l.codes = l.codes[len(l.codes)-1], l.codes[:len(l.codes)-1]
		exp := i.Exp(0)
		l.VisitExp(exp)
		l.codes = append(l.codes, endScope)

		toEnds = append(toEnds, l.pushJmpIfTrue()) // repeat的条件不为true则一直循环 不像for-while
		l.pushJmp().Unary = startIndex + 1         // 避免重复开启scope

		var untilEnd = l.GetNextCodeIndex()

		f()
		// 设置解析的 block 中没有设置过的 break
		l.exitWhileContext(untilEnd + 1)

		// 设置条件自带的 toEnd 位置
		for _, toEnd := range toEnds {
			if toEnd != nil {
				toEnd.Unary = untilEnd
			}
		}

		return nil
	}

	if s := i.If(); s != nil {
		conditionExprCnt, blockCnt := 0, 0
		l.VisitExp(i.Exp(conditionExprCnt))
		var jmpfCode = l.pushJmpIfFalse() // 条件不为真 跳转到else分支
		l.VisitBlock(i.Block(blockCnt))
		var jmp = l.pushJmp() // 条件为真跳转到整个if-else的下条语句
		elseIndex := l.GetNextCodeIndex()
		jmpfCode.Unary = elseIndex
		for range i.AllElseIf() {
			conditionExprCnt++
			blockCnt++
			l.VisitExp(i.Exp(conditionExprCnt))
			jmpfCode := l.pushJmpIfFalse()
			l.VisitBlock(i.Block(blockCnt))
			l.codes = append(l.codes, jmp)
			elseIndex := l.GetNextCodeIndex()
			jmpfCode.Unary = elseIndex

		}
		if i.Else() != nil {
			blockCnt++
			l.VisitBlock(i.Block(blockCnt))
		}
		jmp.Unary = l.GetNextCodeIndex()
		return nil
	}

	if s := i.For(); s != nil {
		if i.Namelist() == nil && i.NAME() != nil {
			f := l.SwitchSymbolTableInNewScope("for-numerical", uuid.NewV4().String())
			defer f()
			iterateVarName := i.NAME().GetText()
			// 把var赋值
			iterateVarID := l.currentSymtbl.NewSymbolWithoutName()
			l.pushLeftRef(iterateVarID)
			l.VisitExp(i.Exp(0))
			l.pushOperator(yakvm.OpFastAssign)
			l.pushOpPop()
			// 只计算一次condition
			conditionId := l.currentSymtbl.NewSymbolWithoutName()
			l.pushLeftRef(conditionId)
			l.VisitExp(i.Exp(1))
			// 为了后面可以根据条件判断是否执行第三条语句，我们需要把结果缓存到中间符号中
			l.pushOperator(yakvm.OpFastAssign)
			l.pushOpPop()

			var stepExp lua.IExpContext
			var stepId int
			if i.Exp(2) != nil {
				stepExp = i.Exp(2)
				stepId = l.currentSymtbl.NewSymbolWithoutName()
				l.pushLeftRef(stepId)
				l.VisitExp(stepExp)
				l.pushOperator(yakvm.OpFastAssign)
				l.pushOpPop()
			}
			// for 执行体结束之后应该无条件跳转回开头，重新判断
			// 但是三语句 for ;; 应该是 block 执行解释后执行第三条语句
			l.pushLeftRef(iterateVarID)
			if stepExp != nil { // step
				l.pushRef(iterateVarID)
				l.pushRef(stepId)
				l.pushOperator(yakvm.OpSub)
			} else {
				l.pushRef(iterateVarID)
				l.pushInteger(1, "1")
				l.pushOperator(yakvm.OpSub)
			}
			l.pushOperator(yakvm.OpFastAssign)
			l.pushOpPop()

			innerWhile := l.SwitchSymbolTableInNewScope("for-numerical-while-inner", uuid.NewV4().String())
			innerStartIndex := l.GetNextCodeIndex()
			l.enterWhileContext(innerStartIndex)

			l.pushLeftRef(iterateVarID)
			if stepExp != nil { // step
				l.pushRef(iterateVarID)
				l.pushRef(stepId)
				l.pushOperator(yakvm.OpAdd)
			} else {
				l.pushRef(iterateVarID)
				l.pushInteger(1, "1")
				l.pushOperator(yakvm.OpAdd)
			}
			l.pushOperator(yakvm.OpFastAssign)
			l.pushOpPop()

			var lastAnd, lastAnd1 *yakvm.Code
			if stepExp != nil {
				l.pushRef(stepId)
				l.pushInteger(0, "0")
				l.pushOperator(yakvm.OpGtEq)

				jmptop1 := l.pushJmpIfFalse()

				l.pushRef(iterateVarID)
				l.pushRef(conditionId)
				l.pushOperator(yakvm.OpGt)

				jmpOr := l.pushJmpIfTrue()
				jmptop1.Unary = l.GetNextCodeIndex()

				l.pushRef(stepId)
				l.pushInteger(0, "0")
				l.pushOperator(yakvm.OpLt)

				lastAnd = l.pushJmpIfFalse()

				l.pushRef(iterateVarID)
				l.pushRef(conditionId)
				l.pushOperator(yakvm.OpLt)

				lastAnd1 = l.pushJmpIfFalse()

				jmpOr.Unary = l.GetNextCodeIndex()
				l.pushBreak()
			} else { // default step is 1
				l.pushInteger(1, "1")
				l.pushInteger(0, "0")
				l.pushOperator(yakvm.OpGtEq)

				jmptop1 := l.pushJmpIfFalse()

				l.pushRef(iterateVarID)
				l.pushRef(conditionId)
				l.pushOperator(yakvm.OpGt)

				jmpOr := l.pushJmpIfTrue()
				jmptop1.Unary = l.GetNextCodeIndex()

				l.pushInteger(1, "1")
				l.pushInteger(0, "0")
				l.pushOperator(yakvm.OpLt)

				lastAnd = l.pushJmpIfFalse()

				l.pushRef(iterateVarID)
				l.pushRef(conditionId)
				l.pushOperator(yakvm.OpLt)

				lastAnd1 = l.pushJmpIfFalse()

				jmpOr.Unary = l.GetNextCodeIndex()
				l.pushBreak()

			}

			lastAnd.Unary = l.GetNextCodeIndex()
			lastAnd1.Unary = l.GetNextCodeIndex()

			fakeIterateVarID, err := l.currentSymtbl.NewSymbolWithReturn(iterateVarName)
			if err != nil {
				l.panicCompilerError(autoCreateSymbolFailed, iterateVarName)
			}
			l.pushLeftRef(fakeIterateVarID) // 注入假变量
			l.pushRef(iterateVarID)
			l.pushOperator(yakvm.OpFastAssign)
			l.pushOpPop()

			l.VisitBlock(i.Block(0))
			l.pushJmp().Unary = innerStartIndex

			var innerWhileEnd = l.GetNextCodeIndex()

			innerWhile()
			l.exitWhileContext(innerWhileEnd)

			return nil
		}
		if i.Namelist() != nil && i.Explist() != nil {
			nameList := strings.Split(i.Namelist().GetText(), ",")
			var nameRef []int
			recoverSymtbl := l.SwitchSymbolTableInNewScope("for-iterate", uuid.NewV4().String())
			defer recoverSymtbl()

			l.VisitExpList(i.Explist())
			iterFuncID := l.currentSymtbl.NewSymbolWithoutName()
			internalStateID := l.currentSymtbl.NewSymbolWithoutName()
			initialValueID := l.currentSymtbl.NewSymbolWithoutName()

			l.pushLeftRef(iterFuncID)
			l.pushLeftRef(internalStateID)
			l.pushLeftRef(initialValueID)
			l.pushListWithLen(3)
			l.pushLocalAssign()

			for _, name := range nameList {
				sym, err := l.currentSymtbl.NewSymbolWithReturn(name)
				if err != nil {
					l.panicCompilerError(constError(fmt.Sprintf("cannot create `%v` variable in generic for reason: ", name)), err.Error())
				}
				nameRef = append(nameRef, sym)
			}

			innerWhile := l.SwitchSymbolTableInNewScope("for-iterate-inner", uuid.NewV4().String())

			innerStartIndex := l.GetNextCodeIndex()
			l.enterWhileContext(innerStartIndex)

			l.pushRef(iterFuncID)
			l.pushRef(internalStateID)
			l.pushRef(initialValueID)
			l.pushCall(2)

			l.pushListWithLen(1)

			for _, ref := range nameRef {
				l.pushLeftRef(ref)
			}

			l.pushListWithLen(len(nameRef))
			l.pushLocalAssign()

			l.pushLeftRef(initialValueID)
			l.pushRef(nameRef[0])
			l.pushOperator(yakvm.OpFastAssign)
			l.pushOpPop()

			l.pushRef(initialValueID)
			l.pushUndefined()
			l.pushOperator(yakvm.OpEq)

			jmp := l.pushJmpIfFalse()
			l.pushBreak()
			jmpIndex := l.GetNextCodeIndex()
			jmp.Unary = jmpIndex

			l.VisitBlock(i.Block(0))
			l.pushJmp().Unary = innerStartIndex

			var innerWhileEnd = l.GetNextCodeIndex()
			innerWhile()
			l.exitWhileContext(innerWhileEnd)

			return nil
		}
	}

	if s, w := i.Function(), i.Local(); s != nil && w == nil {
		l.VisitFuncNameAndBody(i.Funcname(), i.Funcbody())
		return nil
	}

	if s := i.Local(); s != nil {
		if i.Function() != nil {
			// NAME here is necessary no anonymous function allowed
			l.VisitLocalFuncNameAndBody(i.NAME().GetText(), i.Funcbody())
			return nil
		} else {
			list := i.Attnamelist().(*lua.AttnamelistContext)
			nameList := list.AllNAME()
			// fixed: 先不管这个attrib attributeList := list.AllAttrib()
			if expList := i.Explist(); expList != nil {
				l.VisitExpList(expList)
			} else { // 只声明不赋值
				for range nameList {
					l.pushUndefined()
				}
				l.pushListWithLen(len(nameList))
			}

			for index, varName := range nameList {
				l.VisitLocalVarWithName(true, varName.GetText(), list.Attrib(index))
			}
			l.pushListWithLen(len(nameList))
			l.pushLocalAssign()
			return nil
		}
	}

	return nil
}

func (l *LuaTranslator) VisitLastStat(raw lua.ILaststatContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.LaststatContext)
	if i == nil {
		return nil
	}
	if i.Return() != nil {
		if expList := i.Explist(); expList != nil {
			l.VisitExpList(expList)
		}
		l.pushOperator(yakvm.OpReturn)
	}
	if i.Continue() != nil {
		// TODO: 这个continue作为last stat的情况没遇到过 lua按理说没有continue这个关键字 先放着
		panic("TODO")
	}
	if i.Break() != nil {
		if !(l.NowInWhile() || l.NowInRepeat() || l.NowInFor()) {
			log.Warnf("Syntax Error: Break should be in `while`, `repeat`, or `for` loop, it will crash in future")
		}

		l.pushBreak()
		return nil
	}
	return nil
}
