package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_OpcodeFilter_Basic(t *testing.T) {
	code := `
a1 = 11
a2 = 22
f = (p) => {
	return p
}
r = f(a1)
`

	t.Run("opcode_const", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a*?{opcode: const} as $target`, map[string][]string{
			"target": {"11", "22"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("opcode_call", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `r?{opcode: call} as $target`, map[string][]string{
			"target": {"Function-f(11)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("opcode_param", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `p?{opcode: param} as $target`, map[string][]string{
			"target": {"Parameter-p"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}

func TestCondition_OpcodeFilter_WithTopDef(t *testing.T) {
	code := `
f2 = (a1)  => {
  return a1
}
f1 = (a2) => {
	return 1
}
f2(f1(11))
`

	t.Run("filter_itself_is_param", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a1?{opcode: param} as $target`, map[string][]string{
			"target": {"Parameter-a1"},
		})
	})

	t.Run("filter_then_topdef_contains_param_context", func(t *testing.T) {
		// After top-def actual-param resolution, traversing from a filtered param reaches call-site argument values.
		ssatest.CheckSyntaxFlowContain(t, code, `a1?{opcode: param} #-> * as $target`, map[string][]string{
			"target": {"1"},
		})
	})
}

func TestCondition_ProgramVsValueSource(t *testing.T) {
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

	t.Run("program_source_global_filter_with_and", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, code, `*?{opcode: call && have: 'fa'} as $target`, map[string][]string{
			"target": {"Function-fa(1)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("value_source_filter_with_and", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, code, `r*?{opcode: call && have: 'fa'} as $target`, map[string][]string{
			"target": {"Function-fa(1)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}
