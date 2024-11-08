package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"gotest.tools/v3/assert"
)

func Test_Phi_WithGoto(t *testing.T) {
	code := `package main

		func main() {
			a := 1
			if a > 1 {
				a = 5
				goto end
			}else{
				b := a // not phi
		end:
				c := a // phi
			}
		}
`
	ssatest.CheckWithName("phi-with-goto", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("c as $c").GetValues("c")
		nophis := prog.SyntaxFlow("b as $b").GetValues("b")
		phi := phis[0]
		nophi := nophis[0]

		_, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		_, ok = ssa.ToPhi(nophi.GetSSAValue())
		if ok {
			t.Fatal("is phi")
		}

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

}

func Test_Phi_WithGoto_inLoop(t *testing.T) {
	code := `package main

		func println(){}

		func main() {
			a := 1
			for i := 0; i < 10; i++ {
				if i == 1{
					a = 2
					goto label1
				}
			}
			println(a)
			label1:
			println(a)
		}
`
	ssatest.CheckWithName("phi-with-goto-in-loop", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("println(* as $a,)").GetValues("a")

		phi := phis[0]
		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		phi = phis[1]
		targetIns, ok = ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		conds = targetIns.GetControlFlowConditions()
		assert.Equal(t, 0, len(conds)) /* if语句的scope被合并到globel了 */

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))
}

func Test_Phi_WithReturn(t *testing.T) {
	code := `package main

	func main(p int) {
		a := 1
		var u int
		if true {
			return
		}
		b := a
		c := p
		d := u
	}
`
	ssatest.CheckWithName("phi-with-return", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("b as $b").GetValues("b")
		phi := phis[0]

		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

	ssatest.CheckWithName("phi-with-return-undefined", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("d as $d").GetValues("d")
		phi := phis[0]

		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

	ssatest.CheckWithName("phi-with-return-with-param", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		ret := prog.SyntaxFlow("c as $c").GetValues("c")[0]
		_, ok := ssa.ToPhi(ret.GetSSAValue())
		if !ok {
			t.Fatal("It shouldn be phi here")
		}
		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

	ssatest.CheckWithName("phi-with-return-syntaxflow", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("b #{until: `* ?{opcode: phi}`}-> * as $b; check $b;").GetValues("b")
		phi := phis[0]

		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))
}

func Test_MemberCall_WithPhi(t *testing.T) {
	code := `package main
	func main() {
		a := function1()
		if b {
			a = function2()
		}
		
		a.test()
	}
`
	t.Run("member-call-with-phi", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		function1 <getCall> as $entry
		$entry.test as $output
`, map[string][]string{
			"entry":  {"Undefined-function1()"},
			"output": {"Undefined-a.test(valid)"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	code = `package main

	func main() {
		a := function1()
		if b {
			if c {
				a = function2()
			}else{
				a = function3()
			}
			a.test()
		}
		a.test()
	}
`
	t.Run("member-call-with-phi-ex", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		a as $entry
		$entry.test as $output
`, map[string][]string{
			"entry": {"Undefined-function1()",
				"Undefined-function2()",
				"Undefined-function3()",
				"phi(a)[Undefined-function2(),Undefined-function3()]",
				"phi(a)[phi(a)[Undefined-function2(),Undefined-function3()],Undefined-function1()]",
			},
			"output": {"Undefined-a.test(valid)", "Undefined-a.test(valid)"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}
