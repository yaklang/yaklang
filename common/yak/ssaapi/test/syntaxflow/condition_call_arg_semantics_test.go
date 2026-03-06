package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_CallArg_Semantics_CallWideVsPerArg(t *testing.T) {
	code := `
f = (p) => {
  a(p, "a")
  a(p, "b")
  a("a", "b")
}
f(1)
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		// Call-wide semantics: exists a param arg AND exists a constant string "a" somewhere in the args.
		// Use equality rather than `have:"a"` to avoid accidental matches like "Parameter-...".
		callWide, err := prog.SyntaxFlowWithError(`a?(opcode:param && =="a") as $result`)
		require.NoError(t, err)
		require.Equal(t, 1, callWide.GetValues("result").Len())
		require.Contains(t, callWide.GetValues("result")[0].String(), `a(Parameter-p,"a")`)

		// Per-arg semantics: exists an arg that is both a param AND matches "a".
		perArg, err := prog.SyntaxFlowWithError(`a?(*?{opcode:param && =="a"}) as $result`)
		require.NoError(t, err)
		require.Equal(t, 0, perArg.GetValues("result").Len())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}
