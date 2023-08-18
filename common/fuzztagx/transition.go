package fuzztagx

import (
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

type transition struct {
	accept  func(ctx *DataContext) bool
	toState state
}

var stateTransMap map[state][]transition

var actionMap = make(map[state]func(ctx *DataContext))

func init() {
	actionMap[stateStart+stateLeftBrace] = func(ctx *DataContext) { // OnTagStart
		ctx.PushData(NewStringNode(ctx.token))
		ctx.PushToStack(&Tag{})
	}
	actionMap[stateRightParen+stateRightBrace] = func(ctx *DataContext) { // OnTagEnd
		tag, _ := ctx.Pop()
		if ctx.stack.IsEmpty() {
			ctx.PushData(tag.(Node))
		}
	}
	actionMap[stateEmptyRight+stateRightBrace] = actionMap[stateRightParen+stateRightBrace]

	actionMap[stateRightBrace+stateRightParen] = func(ctx *DataContext) {
		if ctx.stack.IsEmpty() {
			ctx.transOk = false
			return
		} else {
			ctx.Pop()
		}
	}
	actionMap[stateEmptyLeft+stateMethod] = func(ctx *DataContext) { // OnMethodStart
		newMethod := &FuzzTagMethod{methodCtx: ctx.methodCtx}
		node := ctx.stack.Peek()
		switch ret := node.(type) {
		case *Tag:
			ret.Nodes = append(ret.Nodes, newMethod)
		case *FuzzTagMethod:
			ret.params = append(ret.params, newMethod)
		}
		ctx.PushToStack(newMethod)
	}
	actionMap[stateEmptyRight+stateMethod] = actionMap[stateEmptyLeft+stateMethod]
	actionMap[stateLeftBrace+stateMethod] = actionMap[stateEmptyLeft+stateMethod]

	actionMap[stateMethod+stateLeftParen] = func(ctx *DataContext) { // OnMethodEnd
		ctx.stack.Peek().(*FuzzTagMethod).name = ctx.token
	}
	actionMap[stateMethod+stateRightBrace] = func(ctx *DataContext) { // OnMethodEnd
		method, _ := ctx.Pop()
		method.(*FuzzTagMethod).name = ctx.token
		itag, _ := ctx.Pop()
		tag := itag.(*Tag)
		ctx.PushData(tag)
	}
	//actionMap[stateLeftParen+stateParam] = func(ctx *DataContext) { // OnParamStart
	//	ctx.preIndex = ctx.currentIndex
	//}
	actionMap[stateLeftParen+stateLeftBrace] = func(ctx *DataContext) { // OnTagParamStart
		newTag := &Tag{}
		ctx.stack.Peek().(*FuzzTagMethod).params = append(ctx.stack.Peek().(*FuzzTagMethod).params, newTag)
		ctx.PushToStack(newTag)
	}
	actionMap[stateParam+stateLeftBrace] = func(ctx *DataContext) { // OnTagParamStart
		newTag := &Tag{}
		ctx.stack.Peek().(*FuzzTagMethod).params = append(ctx.stack.Peek().(*FuzzTagMethod).params, NewStringNode(ctx.token), newTag)
		ctx.PushToStack(newTag)
	}
	actionMap[stateParam+stateRightParen] = func(ctx *DataContext) { // OnParamEnd
		ctx.stack.Peek().(*FuzzTagMethod).params = append(ctx.stack.Peek().(*FuzzTagMethod).params, NewStringNode(ctx.token))
		ctx.Pop()
	}
	actionMap[stateParam+stateLeftParen] = func(ctx *DataContext) { // OnParamEnd
		newMethod := &FuzzTagMethod{methodCtx: ctx.methodCtx, name: ctx.token}
		ctx.stack.Peek().(*FuzzTagMethod).params = append(ctx.stack.Peek().(*FuzzTagMethod).params, newMethod)
		ctx.PushToStack(newMethod)
	}
	actionMap[stateLeftParen+stateRightParen] = func(ctx *DataContext) { // OnParamEnd
		ctx.Pop()
	}

	actionMap[stateLeftBrace+stateExpressionStart] = func(ctx *DataContext) { // OnExpressionStart
		ctx.stack.Peek().(*Tag).IsExpTag = true
		ctx.deep++
	}
	actionMap[stateExpression+stateRightBrace] = func(ctx *DataContext) { // OnExpressionEnd
		itag, _ := ctx.Pop()
		tag := itag.(*Tag)
		exp := ctx.token
		//exp = exp[:len(exp)-1]
		tag.Nodes = append(tag.Nodes, NewExpressionNode(exp))
		if ctx.stack.IsEmpty() {
			ctx.PushData(tag)
		}
	}
	actionMap[stateRightParen+stateNone] = func(ctx *DataContext) { // pda实现
		ctx.currentIndex--
		switch ctx.stack.Peek().(type) {
		case *FuzzTagMethod:
			ctx.toState = stateParam
		case *Tag:
			ctx.toState = stateLeftBrace
		default:
			panic("unexpect")
		}
	}
	actionMap[stateRightBrace+stateNone] = func(ctx *DataContext) { // pda实现
		ctx.currentIndex--
		node := ctx.stack.Peek()
		if node == nil {
			ctx.toState = stateStart
			return
		}
		switch node.(type) {
		case *FuzzTagMethod:
			ctx.toState = stateParam
		default:
			panic("unexpect")
		}
	}
	stateTransMap = map[state][]transition{
		stateStart:           {{StringLeftBrace(), stateLeftBrace}, {CharAccepter(""), stateStart}},
		stateLeftBrace:       {{CharAccepter("="), stateExpressionStart}, {CharAccepter(" \r\n"), stateEmptyLeft}, {CharIdentify(), stateMethod}},
		stateExpressionStart: {{StringRightBrace(), stateRightBrace}, {CharAccepter(""), stateExpression}},
		stateExpression:      {{StringRightBrace(), stateRightBrace}, {CharAccepter(""), stateExpression}},
		stateEmptyLeft:       {{CharAccepter(" \r\n"), stateEmptyLeft}, {CharIdentify(), stateMethod}},
		stateMethod:          {{CharAccepter("("), stateLeftParen}, {CharIdentify(), stateMethod}, {StringRightBrace(), stateRightBrace}, {CharAccepter(" \n"), stateMethodEmpty}},
		stateMethodEmpty:     {{CharAccepter("("), stateLeftParen}, {CharAccepter(" \n"), stateMethodEmpty}},
		stateLeftParen:       {{StringLeftBrace(), stateLeftBrace}, {CharAccepter(")"), stateRightParen}, {CharAccepter(""), stateParam}},
		stateParam:           {{StringLeftBrace(), stateLeftBrace}, {CharAccepter(")"), stateRightParen}, {CharAccepter("("), stateLeftParen}, {CharAccepter(""), stateParam}},
		stateRightParen:      {{CharAccepter(" \r\n"), stateEmptyRight}, {StringRightBrace(), stateRightBrace}, {CharAccepter(""), stateNone}},
		stateEmptyRight:      {{CharAccepter(" \r\n"), stateEmptyRight}, {CharIdentify(), stateMethod}, {StringRightBrace(), stateRightBrace}},
		stateRightBrace:      {{CharAccepter(")"), stateRightParen}, {CharAccepter(""), stateNone}},
	}
}
func CharAccepter(s string) func(ctx *DataContext) bool {
	return func(ctx *DataContext) bool {
		if ctx.currentByte == '\\' && (s == "(" || s == ")") { // 小括号接收器需要转义
			bytSource := []byte(ctx.source)
			bytSource[ctx.currentIndex] = 0
			ctx.source = string(bytSource)
			ctx.currentIndex++
			ctx.currentByte = ctx.source[ctx.currentIndex]
			ctx.transOk = true
			ctx.toState = stateParam
		} else {
			if s == "" {
				ctx.transOk = true
			} else {
				ctx.transOk = strings.Contains(s, string(ctx.currentByte))
			}
		}
		if v, ok := actionMap[ctx.currentState+ctx.toState]; ctx.transOk {
			if ctx.currentState != ctx.toState {
				ctx.token = GetToken(ctx.source[ctx.preIndex:ctx.currentIndex])
				ctx.preIndex = ctx.currentIndex
			}
			if ok {
				v(ctx)
			}
		}
		return ctx.transOk
	}
}
func StringLeftBrace() func(ctx *DataContext) bool {
	return func(ctx *DataContext) bool {
		for i := 0; i < len(ctx.source); i++ {
			if ctx.source[ctx.currentIndex+i] == '{' {
				continue
			} else {
				if i > 1 {
					ctx.currentIndex += i - 1
					ctx.token = GetToken(ctx.source[ctx.preIndex : ctx.currentIndex-1])
					ctx.preIndex = ctx.currentIndex + 1
					ctx.transOk = true
					if v, ok := actionMap[ctx.currentState+ctx.toState]; ok {
						v(ctx)
					}
					ctx.SetIndex(ctx.currentIndex)
					return ctx.transOk
				} else {
					return false
				}
			}
		}
		return false
	}
}
func StringRightBrace() func(ctx *DataContext) bool {
	return func(ctx *DataContext) bool {
		for i := 0; i < len(ctx.source); i++ {
			if ctx.source[ctx.currentIndex+i] == '}' {
				if i == 1 {
					ctx.currentIndex += i
					ctx.token = GetToken(ctx.source[ctx.preIndex : ctx.currentIndex-1])
					ctx.preIndex = ctx.currentIndex + 1
					ctx.transOk = true
					if v, ok := actionMap[ctx.currentState+ctx.toState]; ok {
						v(ctx)
					}
					ctx.SetIndex(ctx.currentIndex)
					return ctx.transOk
				} else {
					continue
				}
			}
			return false
		}
		return false
	}
}
func CharIdentify() func(ctx *DataContext) bool {
	return func(ctx *DataContext) bool {
		ctx.transOk = utils.MatchAllOfRegexp(string(ctx.currentByte), "[a-zA-Z0-9_:-]")
		if v, ok := actionMap[ctx.currentState+ctx.toState]; ctx.transOk {
			if ctx.currentState != ctx.toState {
				ctx.token = GetToken(ctx.source[ctx.preIndex:ctx.currentIndex])
				ctx.preIndex = ctx.currentIndex
			}
			if ok {
				v(ctx)
			}
		}
		return ctx.transOk
	}
}
func GetToken(token string) string {
	res := strings.Builder{}
	for _, ch := range []byte(token) {
		if ch == 0 {
			continue
		}
		res.WriteByte(ch)
	}
	return res.String()
}
