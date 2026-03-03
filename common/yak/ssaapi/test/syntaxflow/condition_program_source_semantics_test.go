package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_ProgramSourceSemantics(t *testing.T) {
	code := `
fa = (p) => {
	return p
}
fb = (p) => {
	return p
}
r1 = fa(1)
r2 = fb(2)
`

	t.Run("and_call_have_fa", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, code, `*?{opcode: call && have: 'fa'} as $target`, map[string][]string{
			"target": {"Function-fa(1)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("have_fb_is_broad", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `*?{have: 'fb'} as $target`, map[string][]string{
			"target": {"Function-fb", "Function-fb(2)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("and_narrows_from_have_to_call", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `*?{have: 'fb' && opcode: call} as $target`, map[string][]string{
			"target": {"Function-fb(2)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("or_unions_two_call_branches", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, code, `*?{(opcode: call && have: 'fa') || (opcode: call && have: 'fb')} as $target`, map[string][]string{
			"target": {"Function-fa(1)", "Function-fb(2)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}
