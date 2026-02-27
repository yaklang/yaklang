package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_ProgramSourceShouldReturnMatchedValues(t *testing.T) {
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

	ssatest.CheckSyntaxFlowContain(t, code, `*?{opcode: call && have: 'fa'} as $target`, map[string][]string{
		"target": {"Function-fa(1)"},
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}
