package php2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func (b *builder) SetRangeFromTerminalNode(node antlr.TerminalNode) func() {
	return b.SetRange(antlr4util.NewToken(node))
}

func (b *builder) SetRange(token antlr4util.CanStartStopToken) func() {
	r := antlr4util.GetRange(b.ir.SourceCode, token)
	if r == nil {
		return func() {}
	}
	backup := b.ir.CurrentRange
	b.ir.CurrentRange = r

	return func() {
		b.ir.CurrentRange = backup
	}
}
