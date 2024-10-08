package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"gotest.tools/v3/assert"
)

func Test_Cross_DoubleSideEffect(t *testing.T) {
	code := `package main

func main() {
	var a = 0
	f1 := func() {
		a = 1
	}
	f2 := func() {
	    f1()
	}
	f2()
}
`
	ssatest.CheckWithName("side-effect: f1->f2", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("f1 as $a").GetValues("a")
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun2.SideEffects))

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

}

func Test_Cross_DoubleSideEffect_lower(t *testing.T) {
	code := `package main

func main() {
	var a = 0
	f2 := func() {
		f1 := func() {
			a = 1
		}
	    f1()
	}
	f2()
}
`
	ssatest.CheckWithName("side-effect: f1->f2", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("f1 as $a").GetValues("a")
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun1.SideEffects))

		fun2, ok := ssa.ToFunction(b[0].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun2.SideEffects))

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))

}
