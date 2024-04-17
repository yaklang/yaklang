package java2ssa

import (
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func (y *builder) SetRange(token antlr4util.CanStartStopToken) func() {
	r := antlr4util.GetRange(y.GetEditor(), token)
	backup := y.CurrentRange
	y.CurrentRange = r

	return func() {
		y.CurrentRange = backup
	}
}
