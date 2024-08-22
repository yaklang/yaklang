package syntaxflow

import (
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestSideEffectAndMask_Recursive(t *testing.T) {
	t.Run("mask", func(t *testing.T) {
		code := `
var a = 3
b = () => {
	a ++
}
if c {
	b()
}
e = a
		`
		ssatest.CheckSyntaxFlow(t, code,
			`e #-> * as $target`,
			map[string][]string{
				"target": {"1", "3", "Undefined-c"},
			})
	})
}

func TestObject_Recursive(t *testing.T) {
	t.Run("object", func(t *testing.T) {
		code := `
		a = {}
		b = () => {
			a.b = 333333
		}
		b()
		c = a.b;
		`
		ssatest.CheckSyntaxFlowContain(t, code,
			`c #-> * as $target`,
			map[string][]string{
				"target": {"333333"},
			},
		)
	})

	t.Run("object mark self", func(t *testing.T) {
		code := `
			a = {}; 
			a.b = 1; 
			a.c = 3; 
			d = a.c + a.b
		`
		ssatest.CheckSyntaxFlow(t, code,
			`d #-> * as $target`,
			map[string][]string{
				"target": {"1", "3"},
			},
		)
	})

	t.Run("object mark self, bottom user", func(t *testing.T) {
		// func Test_Yaklang_BottomUser(t *testing.T) {
		code := `
		f = () =>{
			a = 11
			return a
		}
		f2 = (i) => {
			println(i)
		}
		t = f()
		f2(t)
		`
		ssatest.CheckSyntaxFlow(t, code,
			`a --> * as $target`,
			map[string][]string{
				"target": {
					"FreeValue-println(Parameter-i)",
				},
			},
		)
	})
}

func TestFunctionCall_REcursive(t *testing.T) {
	t.Run("function call", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (a) => {
			return a
		}
		f2 = (a)  => {
			target = f(a)
			f2(target)
		}
		`, `
		target #-> * as $target
		`, map[string][]string{
			"target": {"FreeValue-f(Parameter-a)"},
		})
	})
}

func TestFunctionCall_Recursive2(t *testing.T) {
	t.Run("function call", func(t *testing.T) {
		ssatest.Check(t, `f2 = (a)  => {
			f2(a)
		}`, func(prog *ssaapi.Program) error {
			result := prog.SyntaxFlow("a?{opcode: param} #-> * as $target")
			result.Show()
			return nil
		})
	})
}
