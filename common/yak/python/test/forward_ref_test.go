package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestPythonForwardRef_ModuleLevelCallLaterDef(t *testing.T) {
	code := `
def cmd_injection_hard(query):
    return cmd_injection_low(query)

def cmd_injection_low(query):
    return query
`
	prog, err := ssaapi.Parse(code, ssaapi.WithLanguage(ssaconfig.PYTHON))
	require.NoError(t, err)

	res, err := prog.SyntaxFlowWithError(`cmd_injection_low as $callee`)
	require.NoError(t, err)

	for _, v := range res.GetValues("callee") {
		require.False(t, v.IsUndefined(),
			"later-defined module function should resolve at call sites in earlier defs; got %s", v.String())
	}
}
