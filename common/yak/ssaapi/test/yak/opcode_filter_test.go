package ssaapi

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestOpcodeFilterTest(t *testing.T) {
	ssatest.Check(t, `a = "a"; a += undefined; c = dump(a)`, func(prog *ssaapi.Program) error {
		values, err := prog.SyntaxFlowWithError(`dump(*?{opcode: '+'} as $params)`)
		if err != nil {
			t.Fatal(err)
		}
		passed := false
		for _, v := range values.GetValues("params") {
			if v.GetBinaryOperator() == string(ssa.OpAdd) {
				passed = true
			}
		}
		if !passed {
			t.Fatal("filter add failed")
		}
		return nil
	})
}
