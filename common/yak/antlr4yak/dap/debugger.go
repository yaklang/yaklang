package dap

import (
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

type DAPDebugger struct {
}

func (d *DAPDebugger) Init() func(g *yakvm.Debugger) {
	return func(g *yakvm.Debugger) {
		// 在第一个opcode执行的时候开始回调
		g.Callback()
	}
}

func (d *DAPDebugger) CallBack() func(g *yakvm.Debugger) {
	return func(g *yakvm.Debugger) {
	}
}

func NewDAPDebugger() *DAPDebugger {
	return &DAPDebugger{}
}
