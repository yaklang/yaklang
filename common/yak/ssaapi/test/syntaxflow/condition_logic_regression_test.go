package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_LogicBoundary(t *testing.T) {
	code := `
a1 = 11
a2 = 22
`

	t.Run("and_match_single", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a*?{opcode: const && have: '11'} as $target`, map[string][]string{
			"target": {"11"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("and_match_empty", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a*?{opcode: const && have: 'not-exist'} as $target`, map[string][]string{
			"target": {},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("or_match_all", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a*?{have: '11' || have: '22'} as $target`, map[string][]string{
			"target": {"11", "22"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("not_with_and", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a*?{!have: '11' && opcode: const} as $target`, map[string][]string{
			"target": {"22"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}

func TestRecursiveConfig_IncludeConditionBoundary(t *testing.T) {
	code := `
b1 = f1(1)
b2 = f2(2)
`

	t.Run("include_and_match_single_path", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `b* #{include:`+"`"+`* ?{have:f1 && opcode:call}`+"`"+`}-> as $result`, map[string][]string{
			"result": {"Undefined-f1"},
		})
	})

	t.Run("include_and_match_empty", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `b* #{include:`+"`"+`* ?{have:'not-exist' && opcode:call}`+"`"+`}-> as $result`, map[string][]string{
			"result": {},
		})
	})
}
