package fuzztagx

import (
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

type state string

const (
	stateLeftBrace0  state = "{0"
	stateLeftBrace1  state = "{1"
	stateEmptyLeft   state = "emptyLeft"
	stateEmptyRight  state = "emptyRight"
	stateMethod      state = "method"
	stateLeftParen   state = "("
	stateParam       state = "param"
	stateRightParen  state = ")"
	stateRightBrace0 state = "}0"
	stateRightBrace1 state = "}1"
	stateStart       state = "start"
)

type transition struct {
	accept  func(byte) bool
	toState state
	cb      func(ctx *DataContext, s string) // 当状态转换时会传入上一个状态匹配到的字符串
}

func CharAccepter(s string) func(byte) bool {
	return func(b2 byte) bool {
		if s == "" {
			return true
		}
		return strings.Contains(s, string(b2))
	}
}

type DataContext struct {
	data       []any
	currentTag *FuzzTag
}

func (d *DataContext) Generate() ([]string, error) {
	return nil, nil
}
func (d *DataContext) PushData(data any) {
	d.data = append(d.data, data)
}

var stateTransMap map[state][]transition

func init() {
	var fuzztagStartCB = func(ctx *DataContext, s string) {
		ctx.currentTag = &FuzzTag{}
	}
	stateTransMap = map[state][]transition{
		stateStart: {{CharAccepter("{"), stateLeftBrace0, func(ctx *DataContext, s string) {
			ctx.PushData(s)
		}}},
		stateLeftBrace0: {{CharAccepter("{"), stateLeftBrace1, nil}},
		stateLeftBrace1: {{CharAccepter(" \r\n"), stateEmptyLeft, nil}, {CharAccepter(""), stateMethod, fuzztagStartCB}},
		stateEmptyLeft:  {{CharAccepter(" \r\n"), stateEmptyLeft, nil}, {CharAccepter(""), stateMethod, fuzztagStartCB}},
		stateMethod: {{CharAccepter("("), stateLeftParen, func(ctx *DataContext, s string) {
			ctx.currentTag.Method = append(ctx.currentTag.Method, FuzzTagMethod{
				name: s,
			})
		}}},
		stateLeftParen: {{CharAccepter(""), stateParam, nil}},
		stateParam: {{CharAccepter(")"), stateRightParen, func(ctx *DataContext, s string) {
			ctx.currentTag.Method[len(ctx.currentTag.Method)-1].param = s
		}}},
		stateRightParen: {{CharAccepter(" \r\n"), stateEmptyRight, nil}, {CharAccepter(""), stateRightBrace0, nil}},
		stateEmptyRight: {{CharAccepter(" \r\n"), stateEmptyRight, nil}, {CharAccepter(""), stateRightBrace0, nil}},
		stateRightBrace0: {{CharAccepter("}"), stateRightBrace1, func(ctx *DataContext, s string) {
			ctx.PushData(ctx.currentTag)
		}}},
		stateRightBrace1: {{CharAccepter(""), stateStart, nil}},
	}
}

func ExecuteWithStringHandler(raw interface{}, method map[string]interface{}) ([]string, error) {
	rawCode := utils.InterfaceToString(raw)

	// 在状态切换cb中的上下文，用于储存结果
	ctx := &DataContext{}

	// pda 使用栈让括号平衡
	stack := utils.NewStack()
	stack.Push(stateStart)
	resolutionMap := map[state]state{stateLeftParen: stateRightParen, stateLeftBrace1: stateRightBrace1, stateLeftBrace0: stateRightBrace0}
	checkState := func(s state) {
		peek := stack.Peek()
		success := false
		if peek != nil {
			if v, ok := resolutionMap[peek.(state)]; ok && v == s {
				stack.Pop()
				success = true
			}
		}
		if !success {
			stack.Push(s)
		}
	}

	// fsm 词法解析
	preI := 0
	currentState := stateStart
	for i := 0; ; i++ {
		if i >= len(rawCode) {
			ctx.PushData(rawCode[preI:i])
			break
		}
		var b = rawCode[i]
		v, ok := stateTransMap[currentState]
		if !ok {
			return nil, utils.Errorf("not defined state: %v", stack.Peek())
		}
		for _, trans := range v {
			if trans.accept(b) {
				if trans.cb != nil {
					trans.cb(ctx, rawCode[preI:i])
				}
				preI = i
				currentState = trans.toState
				checkState(trans.toState)
				break
			}
		}
	}

	return ctx.Generate()
}
