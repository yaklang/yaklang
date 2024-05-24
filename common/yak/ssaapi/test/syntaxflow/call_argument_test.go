package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCallArgument_AsVariable(t *testing.T) {

	check := func(t *testing.T, sf string, expect map[string][]string) {
		code := `
		a = 1
		b = 2
		c = 3
		f(a, b, c)
		`
		ssatest.CheckSyntaxFlow(t, code, sf, expect)
	}

	t.Run("test call self", func(t *testing.T) {
		check(t,
			`f() as $call`, map[string][]string{
				"call": {"Undefined-f(1,2,3)"},
			})
	})

	t.Run("test all argument ", func(t *testing.T) {
		check(t,
			`f(* as $a)`,
			map[string][]string{
				"a": {"1", "2", "3"},
			},
		)
	})

	t.Run("test first argument as variable", func(t *testing.T) {
		check(t,
			`f(* as $a,,)`,
			map[string][]string{
				"a": {"1"},
			},
		)
	})

	t.Run("first  argument ignore other", func(t *testing.T) {
		check(t,
			`f(* as $a, )`,
			map[string][]string{
				"a": {"1"},
			},
		)
	})

	t.Run("test first and second argument ", func(t *testing.T) {
		check(t, `f(* as $a, * as $b)`, map[string][]string{
			"a": {"1"},
			"b": {"2"},
		})
	})

	t.Run("test first and third argument ", func(t *testing.T) {
		check(t, `f(* as $a, ,* as $c)`, map[string][]string{
			"a": {"1"},
			"c": {"3"},
		})
	})
}

func TestCall_SideEffect(t *testing.T) {
	t.Run("yaklang sideEffect", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t,
			`
	a = 1 
	f = (i) => {
		a = i
	}
	f(12)
	print(a)
	`,
			`print(* #-> * as $target)`,
			map[string][]string{
				"target": {"12"},
			},
		)
	})

	t.Run("java class", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t,
			`
		class A {
			int a;
			void set(int i) {
				this.a = i;
			}
			int get() {
				return this.a;
			}
		}
		class Main{
			void main() {
				a = new A();
				a.set(12);
				print(a.get());
			}
		}
		`,
			`print(* #-> * as $target)`,
			map[string][]string{
				"target": {"12"},
			},
			ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
}
