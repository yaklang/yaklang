package ssaapi

import (
<<<<<<< HEAD
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
=======
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
>>>>>>> fd8428263 (add HandlerReturnPhi and add test)
)

func TestIFReturnPhi(t *testing.T) {
	ssatest.Check(t, `
a = 1
if b{ return}
d = dump(a)
`, func(prog *ssaapi.Program) error {
<<<<<<< HEAD
=======
		prog.Show()
>>>>>>> fd8428263 (add HandlerReturnPhi and add test)
		prog.Ref("d").GetTopDefs().Show()
		result, err := prog.SyntaxFlowWithError("d #{until: `* ?{opcode: phi}`}-> * as $result; check $result;")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Errors) > 0 {
			t.Fatal(result.Errors)
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}
