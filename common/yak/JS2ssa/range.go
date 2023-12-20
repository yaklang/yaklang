package js2ssa

import (
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func (b *astbuilder) SetRange(token antlr4util.CanStartStopToken) func() {
	r := antlr4util.GetRange(token)
	backup := b.CurrentRange
	b.CurrentRange = r

	return func() {
		b.CurrentRange = backup
	}
}
