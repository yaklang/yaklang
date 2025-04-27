package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestCAllFilter(t *testing.T) {
	code := `
a(1,b)
a(2,2)
`
	t.Run("test get first param is const", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a(*?{opcode: const},) as $call`, map[string][]string{
			"call": {"Undefined-a(1,Undefined-b)", "Undefined-a(2,2)"},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test get params have const", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a(*?{opcode: const}) as $call`, map[string][]string{
			"call": {"Undefined-a(1,Undefined-b)", "Undefined-a(2,2)"},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
	t.Run("test param is all const", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `a(*?{opcode: const},*?{opcode: const}) as $call`, map[string][]string{
			"call": {"Undefined-a(2,2)"},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
}
