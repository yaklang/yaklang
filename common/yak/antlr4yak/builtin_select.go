package antlr4yak

import "github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

func (e *Engine) yakBuiltinSelect(cases []*yakvm.Value) []any {
	frame := e.vm.CurrentFM()
	if frame == nil {
		panic("select current frame is empty")
	}
	return frame.SelectChannels(cases)
}
