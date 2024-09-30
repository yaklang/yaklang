package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"gotest.tools/v3/assert"
)

func Test_Cross_Function(t *testing.T) {
	t.Run("multiple parameter", func(t *testing.T) {
		code := `package main

			func f1(a,b int) int {
				return a
			}

			func f2(a,b int) int {
				return b
			}

			func main(){
				c := f1(1,2)
				d := f2(1,2)
			}
		`
		ssatest.CheckSyntaxFlow(t, code, `
		c #-> as $c_def
		d #-> as $d_def
		`, map[string][]string{
			"c_def": {"1"},
			"d_def": {"2"},
		},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("multiple return first", func(t *testing.T) {
		ssatest.Check(t, `package main

			func f1() (int,int) {
				return 1,2
			}

			func main(){
				c,d := f1()
			}
		`,
			ssatest.CheckTopDef_Equal("c", []string{"1"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("multiple return second", func(t *testing.T) {
		ssatest.Check(t, `package main

			func f1() (int,int) {
				return 1,2
			}

			func main(){
				c,d := f1()
			}
		`,
			ssatest.CheckTopDef_Equal("d", []string{"2"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("default return", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

		func test()(a int){
		    a = 6
			return
		}

		func main(){
			r := test()
		}
		`, `
		r #-> as $target
		`, map[string][]string{
			"target": {"6"},
		},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("multiple default return first", func(t *testing.T) {
		ssatest.Check(t, `package main

			func f1() (a,b int) {
				a = 1
				b = 2
				return
			}

			func main(){
				c,d := f1()
			}
		`,
			ssatest.CheckTopDef_Equal("c", []string{"1"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("multiple default return second", func(t *testing.T) {
		ssatest.Check(t, `package main

			func f1() (a,b int) {
				a = 1
				b = 2
				return
			}

			func main(){
				c,d := f1()
			}
		`,
			ssatest.CheckTopDef_Equal("d", []string{"2"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func Test_Function_Global(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.Check(t, `package main
		var a = 1

		func main(){
			b := a
		}
		`, ssatest.CheckTopDef_Equal("b", []string{"1"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func Test_Closure(t *testing.T) {
	t.Run("freevalue", func(t *testing.T) {
		code := `package main

		func main(){
			a := 1
			c := func (){
				b := a
			}
		}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`b #-> as $target`,
			map[string][]string{
				"target": {"1"},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("side-effect before", func(t *testing.T) {
		code := `package main

		func main(){
			a := 1
			c := func (){
				a = 2
			}
			show := func(i int){
				num	:= i
			}
			show(a) // 1

			c()
			show(a) // side-effect(2)
		}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`num #-> as $target`,
			map[string][]string{
				"target": {"1", "2"},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}
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

/*
func Test_Cross_DoubleSideEffect_more(t *testing.T) {
	code := `package main

	func main() {
	    var a = 0
		var f1, f2 func()
		f2 = func() {
			f1 = func() {
				f2()
			}
			a = 1
		}
		f1()
	}
	`
	ssatest.CheckWithName("side-effect: f2->f1", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		a := prog.SyntaxFlow("f1 as $a").GetValues("a")
		b := prog.SyntaxFlow("f2 as $b").GetValues("b")

		fun1, ok := ssa.ToFunction(a[1].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun1.SideEffects))

		fun2, ok := ssa.ToFunction(b[2].GetSSAValue())
		if !ok {
			t.Fatal("not function")
		}
		assert.Equal(t, 1, len(fun2.SideEffects))

		return nil
	}, ssaapi.WithLanguage(ssaapi.GO))
}
*/
