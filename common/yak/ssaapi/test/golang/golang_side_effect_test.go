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

func Test_Captured_SideEffect(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		code := `package main

	import "fmt"

	func test() {
		a := 1
		f := func() {
			a = 0
		}
		{
			a := 2
			f()
			b := a // 2 不会被side-effect影响
		}

		c := a // 0 会被side-effect影响
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
		`, map[string][]string{
			"b": {"2"},
			"c": {"0", "1"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("nesting", func(t *testing.T) {
		code := `package main

	import "fmt"

	func test() {
		a := 1
		f := func() {
			a = 0
		}
		{
			a := 2
			{
				a := 3
				f()
				b := a // 3 不会被side-effect影响
			}
			f()
			c := a // 2 不会被side-effect影响
		}
		d := a // 0 会被side-effect影响
	}
		`
		ssatest.CheckSyntaxFlow(t, code, `
			b #-> as $b
			c #-> as $c
			d #-> as $d
		`, map[string][]string{
			"b": {"3"},
			"c": {"2"},
			"d": {"0", "1"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}
