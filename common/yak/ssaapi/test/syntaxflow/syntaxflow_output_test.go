package syntaxflow

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"strings"
	"testing"
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
		result := sfResult.String()
		fmt.Println(result)
		require.True(t, strings.Contains(result, "$b:") && !strings.Contains(result, "$a:"))
		result = sfResult.String(sfvm.WithShowAll(true))
		fmt.Println(result)
		require.True(t, strings.Contains(result, "$b:") && strings.Contains(result, "$a:"))
		result = sfResult.String(sfvm.WithShowAll(true), sfvm.WithShowCode(true))
		fmt.Println(result)
		result = sfResult.String(sfvm.WithShowAll(true), sfvm.WithShowDot(true))
		fmt.Println(result)
		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}
