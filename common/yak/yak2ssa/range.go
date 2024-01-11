package yak2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func (b *astbuilder) SetRangeFromTerminalNode(node antlr.TerminalNode) func() {
	return b.SetRange(antlr4util.NewToken(node))
}

func (b *astbuilder) SetRange(token antlr4util.CanStartStopToken) func() {
	// token :=
	r := antlr4util.GetRange(token)
	if r == nil {
		return func() {}
	}
	backup := b.CurrentRange
	b.CurrentRange = r

	return func() {
		b.CurrentRange = backup
	}
}
