package luaast

import "yaklang/common/yak/antlr4yak/yakvm"

func (l *LuaTranslator) enterForContext(start int) {
	l.forDepthStack.Push(&whileContext{
		startCodeIndex: start,
	})
}

func (l *LuaTranslator) exitForContext(end int) {
	start := l.peekWhileStartIndex()
	if start <= 0 {
		return
	}

	for _, c := range l.codes[start:] {
		if c.Opcode == yakvm.OpBreak && c.Unary <= 0 {
			// 设置 while 开始到结尾的所有语句的 Break Code 的跳转值
			c.Unary = end
		}
	}
	l.forDepthStack.Pop()
}
