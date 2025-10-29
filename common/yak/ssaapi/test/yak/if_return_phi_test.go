package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestIFReturnPhi(t *testing.T) {
	code := `
a = 1
if b{ return}
d = dump(a)
`

	rule := `
d #{until: "* ?{opcode: phi}"}-> * as $result; 
check $result;
`

	ssatest.CheckSyntaxFlow(t, code, rule, map[string][]string{
		"result": {"phi(a)[Undefined-a,1]"},
	}, ssaapi.WithLanguage(ssaconfig.Yak))
}
