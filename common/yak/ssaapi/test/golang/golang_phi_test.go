package ssaapi

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/yaklang"
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

		_, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		_, ok = ssa.ToPhi(nophi.GetSSAInst())
		if ok {
			t.Fatal("is phi")
		}

		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

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
		res := prog.SyntaxFlow("println(* as $a,)", ssaapi.QueryWithEnableDebug())
		res.Show()
		phis := res.GetValues("a")

		for _, phi := range phis {
			targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
			if !ok {
				t.Fatal("not phi")
			}
			conds := targetIns.GetControlFlowConditions()
			if !(len(conds) == 0 || len(conds) == 1) {
				t.Fatal("should be 0 or 1")
			}
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))
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

		targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

	ssatest.CheckWithName("phi-with-return-undefined", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("d as $d").GetValues("d")
		phi := phis[0]

		targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

	ssatest.CheckWithName("phi-with-return-with-param", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		ret := prog.SyntaxFlow("c as $c").GetValues("c")[0]
		_, ok := ssa.ToPhi(ret.GetSSAInst())
		if !ok {
			t.Fatal("It shouldn be phi here")
		}
		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

	ssatest.CheckWithName("phi-with-return-syntaxflow", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("b #{until: `* ?{opcode: phi}`}-> * as $b; check $b;").GetValues("b")
		phi := phis[0]

		targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))
}

func Test_Phi_WithReturn_Extend(t *testing.T) {
	code := `package main

	func main(p int) {
		a := 1
		var u int
		if a == 1 {
			return
		} else if a == 2 {
			return
		} else if a == 3 {
			return
		}
		b := a
		c := p
		d := u
	}
`
	ssatest.CheckWithName("phi-with-return-else-if", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("b as $b").GetValues("b")
		phi := phis[0]

		targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 2, len(conds))

		phis = prog.SyntaxFlow("c as $c").GetValues("c")
		phi = phis[0]

		targetIns, ok = ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds = targetIns.GetControlFlowConditions()
		assert.Equal(t, 2, len(conds))

		phis = prog.SyntaxFlow("d as $d").GetValues("d")
		phi = phis[0]

		targetIns, ok = ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds = targetIns.GetControlFlowConditions()
		assert.Equal(t, 2, len(conds))

		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

	code = `package main

	func main(p int) {
		a := 1
		var u int
		if a == 1 {
			if a == 2 {
				if a == 3 {
					return
				}
			}
		} 
		b := a
		c := p
		d := u
	}
`
	ssatest.CheckWithName("phi-with-return-nested-if", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("b as $b").GetValues("b")
		phi := phis[0]

		targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		phis = prog.SyntaxFlow("c as $c").GetValues("c")
		phi = phis[0]

		targetIns, ok = ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds = targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		phis = prog.SyntaxFlow("d as $d").GetValues("d")
		phi = phis[0]

		targetIns, ok = ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds = targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))

	code = `package main

	func main(p int) {
		a := 1
		var u int
		if a == 1 {
			if a == 2 {
				return
			} else {
				return
			}
		} 
		b := a
		c := p
		d := u
	}
`
	ssatest.CheckWithName("phi-with-return-nested-if-else", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("b as $b").GetValues("b")
		phi := phis[0]

		targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		phis = prog.SyntaxFlow("c as $c").GetValues("c")
		phi = phis[0]

		targetIns, ok = ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds = targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		phis = prog.SyntaxFlow("d as $d").GetValues("d")
		phi = phis[0]

		targetIns, ok = ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds = targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		return nil
	}, ssaapi.WithLanguage(ssaconfig.GO))
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
		function1() as $entry
		$entry.test as $output
`, map[string][]string{
			"entry":  {"Undefined-function1()"},
			"output": {"Undefined-a.test"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
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
		function1() as $entry
		$entry.test as $output
`, map[string][]string{
			"entry":  {"Undefined-function1()"},
			"output": {"Undefined-a.test"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	code = `package main

	func main() {
		a := function1()
		if b {
			a = function2()
			a.test(2)
		}
		a.test(1)
	}
`
	t.Run("member-call-and-param-with-phi", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		function1() as $f1
		function2() as $f2

		$f1.test(, * #-> as $output1) 
		$f2.test(, * #-> as $output2) 
`, map[string][]string{
			"output1": {"1"},
			"output2": {"1", "2"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})

	code = `package main

	func main() {
		a := function1() // 1 5
		if b {
			a.test(1)
			a = function2() // 2 4 5
			if c {
				a.test(2)
				if d {
					a = function3() // nil
				}
				a = function4() // 3 4 5
				a.test(3)
			}
			a.test(4)
		}
		a.test(5)
	}
`
	t.Run("member-call-with-phi-complex", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		function1() as $f1
		function2() as $f2
		function3() as $f3
		function4() as $f4

		$f1.test(, * #-> as $output1) 
		$f2.test(, * #-> as $output2) 
		$f3.test(, * #-> as $output3) 
		$f4.test(, * #-> as $output4) 
`, map[string][]string{
			"output1": {"1", "5"},
			"output2": {"2", "4", "5"},
			"output4": {"3", "4", "5"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})
}

func Test_ImportPackage_WithPhi(t *testing.T) {
	code := `package main

	import "github.com/your/template"

	func main() {
		t, err := template.New().Parse()
		if err != nil {
			t, err = template.New().Parse()
		}
		t.Execute(w, messages)
	}`

	t.Run("import-package-with-phi", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code, `
		template?{<fullTypeName>?{have: 'github.com/your/template'}} as $entry;
		$entry.New() as $new;
		$new.Parse()<getMembers> as $tmp 
		$tmp.Execute(, * #-> as $sink);
		`, map[string][]string{
			"sink": {"Undefined-w", "Undefined-messages"},
		}, ssaapi.WithLanguage(ssaconfig.GO))
	})
}

func Test_PhiType(t *testing.T) {
	code := `package main

	func main() {
		encodePayload,err = codec.AESCBCEncrypt("", "", "")
		if err {
			// panic("codec AES CBC Encrypt error")
            return 
		}	
        print(encodePayload)
	}`

	symbol := yaklang.New().GetFntable()
	opts := make([]ssaconfig.Option, 0)
	tmp := reflect.TypeOf(make(map[string]interface{}))
	for name, item := range symbol {
		itype := reflect.TypeOf(item)
		if itype == tmp {
			opts = append(opts, ssaapi.WithExternLib(name, item.(map[string]interface{})))
		}
	}
	opts = append(opts, ssaapi.WithLanguage(ssaconfig.GO))

	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		res, err := prog.SyntaxFlowWithError(`
		print( * as $para)
		$para<typeName()> as $typeName 
		`)
		require.NoError(t, err)
		typeName := res.GetValues("typeName")
		// typeName
		require.True(t, len(typeName) == 1)
		require.Equal(t, typeName[0].String(), "\"bytes\"")

		return nil
	}, opts...)
}
