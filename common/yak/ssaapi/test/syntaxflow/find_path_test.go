package syntaxflow

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
		results, err := prog.SyntaxFlowWithError(`
		h as $start
		$start #-> *?{<name>?{have: a}} as $end
		alert $start
		alert $end
		`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		require.NotNil(t, results)
		fmt.Println(results.Dump(false))
		start := results.GetValues("start")
		log.Infof("start: %v", start)
		start.ShowDot()
		end := results.GetValues("end")
		log.Infof("end: %v", end)
		end.ShowDot()
		paths := start.GetDataflowPath(end...)
		for _, item := range paths {
			item := ssaapi.Values(item)
			require.Equal(t, item[0].GetSSAInst().GetOpcode(), ssa.SSAOpcodeBinOp)
			require.Equal(t, item[len(item)-1].GetSSAInst().GetOpcode(), ssa.SSAOpcodeConstInst)
			fmt.Println(item.String())
		}
		require.Greater(t, len(paths), 0)
		return nil
	})
}
