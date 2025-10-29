package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCAllFilter(t *testing.T) {
	code := `
a(1,b)
a(2,2)
`
	t.Run("test get first param is const", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a?(*?{opcode: const},) as $call`, map[string][]string{
			"call": {"Undefined-a(1,Undefined-b)", "Undefined-a(2,2)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test get params have const", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a?(*?{opcode: const}) as $call`, map[string][]string{
			"call": {"Undefined-a(1,Undefined-b)", "Undefined-a(2,2)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test param is all const", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a?(*?{opcode: const},*?{opcode: const}) as $call`, map[string][]string{
			"call": {"Undefined-a(2,2)"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}

func TestConditionFilter(t *testing.T) {
	code := `a("b")`
	t.Run("check sf >=", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a?(*?{<='c'}) as $sink`, map[string][]string{
			"sink": {`Undefined-a("b")`},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}
