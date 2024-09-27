package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestSideEffectPassingViaCall(t *testing.T) {
	ssatest.Check(t, `
a = 1
b = () => {
    if c {
        a = 2
    }
}
d = () => {
    b()
	g = a
}
d()
f = a
`, func(prog *ssaapi.Program) error {
		prog.SyntaxFlow("f").Show()
		prog.SyntaxFlow("g").Show()
		return nil
	})
}
