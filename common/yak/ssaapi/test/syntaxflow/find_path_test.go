package syntaxflow

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestFindPathFromSyntaxFlow(t *testing.T) {
	ssatest.Check(t, `
a = 1;
b = (c, d) => {
	return d + 2;
}
g = 3
f = 0
if e {
	f = b(4, a);
}
h = f + g
`, func(prog *ssaapi.Program) error {
		results := prog.SyntaxFlow("h as $start; $start #-> *?{<name>?{have: a}} as $end; alert $start; alert $end")
		fmt.Println(results.Dump(false))
		start := results.GetValues("start")
		start.ShowDot()
		end := results.GetValues("end")
		paths := start.GetPaths(end)
		for _, item := range paths {
			assert.Equal(t, item[0].GetSSAValue().GetOpcode(), ssa.SSAOpcodeBinOp)
			assert.Equal(t, item[len(item)-1].GetSSAValue().GetOpcode(), ssa.SSAOpcodeConstInst)
			fmt.Println(item.String())
		}
		assert.Greater(t, len(paths), 0)
		return nil
	})
}
