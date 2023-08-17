package fuzztagx

import (
	"github.com/yaklang/yaklang/common/utils"
)

type state string

const (
	stateLeftBrace       state = "{{"
	stateEmptyLeft       state = "emptyLeft"
	stateEmptyRight      state = "emptyRight"
	stateMethod          state = "method"
	stateLeftParen       state = "("
	stateParam           state = "param"
	stateRightParen      state = ")"
	stateRightBrace      state = "}}"
	stateStart           state = "start"
	stateExpressionStart state = "{{="
	stateExpression      state = "expression"
	stateNone            state = "None"
)

func Parse(raw interface{}, methodCtx *MethodContext) ([]Node, error) {
	rawCode := utils.InterfaceToString(raw)
	ctx := NewDataContext(rawCode) // 在状态切换cb中的上下文
	ctx.methodCtx = methodCtx
	// 词法解析
	currentState := stateStart
	trans := func() {
		defer func() {
			if e := recover(); e != nil {
				//解析出错时重置状态
				currentState = stateStart
				i := -1
				for !ctx.stack.IsEmpty() {
					_, i = ctx.Pop()
				}
				if i != -1 {
					ctx.PushData(NewStringNode(ctx.source[i:ctx.currentIndex]))
				}
			}
		}()
		ctx.unscanstr = rawCode[ctx.currentIndex:]
		var b = rawCode[ctx.currentIndex]
		ctx.currentByte = b
		ctx.currentState = currentState
		v, ok := stateTransMap[currentState]
		if !ok {
			panic(utils.Errorf("not defined state: %v", currentState))
		}
		ok = false
		for _, trans := range v {
			ctx.toState = trans.toState
			if trans.accept(ctx) {
				ok = true
				currentState = ctx.toState
				break
			}
		}
		if !ok {
			panic(utils.Errorf("unexpect char `%v` on index %d", string(ctx.currentByte), ctx.currentIndex))
		}
	}
	for ; ctx.currentIndex < len(rawCode); ctx.currentIndex++ {
		trans()
	}
	ctx.PushData(NewStringNode(ctx.source[ctx.preIndex:]))
	return ctx.data, nil
}
