package syntaxflow

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSfOutput(t *testing.T) {
	code := `
a = 1
b = 2
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		sfResult, err := prog.SyntaxFlowWithError(`
a as $a
b as $b
alert $b
`)
		require.NoError(t, err)
		// default
		result := sfResult.String()
		fmt.Println(result)
		require.Contains(t, result, "$b:")
		require.NotContains(t, result, "$a:")

		// show all
		result = sfResult.String(sfvm.WithShowAll(true))
		fmt.Println(result)
		require.Contains(t, result, "$b:")
		require.Contains(t, result, "$a:")

		// with code
		result = sfResult.String(sfvm.WithShowCode(true))
		fmt.Println(result)
		require.Contains(t, result, "b = 2")

		// with dot
		result = sfResult.String(sfvm.WithShowDot(true))
		fmt.Println(result)
		require.Contains(t, result, "strict digraph")
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}
