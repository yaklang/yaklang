package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestOpcodeFilterTest(t *testing.T) {
	ssatest.Check(t, `a = "a"; a += "b"; c = dump(a)`, func(prog *ssaapi.Program) error {
		values, err := prog.SyntaxFlowWithError(`dump(*?{opcode: '+'})`)
		if err != nil {
			t.Fatal(err)
		}
		values.Show()
		return nil
	})
}
