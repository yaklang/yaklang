package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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
			"b": {"2", "3"},
		})
	})

	t.Run("test second and continue ", func(t *testing.T) {
		check(t, `f(, * as $b)`, map[string][]string{
			"b": {"2", "3"},
		})
	})

	t.Run("test first and third argument ", func(t *testing.T) {
		check(t, `f(* as $a, ,* as $c)`, map[string][]string{
			"a": {"1"},
			"c": {"3"},
		})
	})
	t.Run("test no function get argument", func(t *testing.T) {
		check(t, `b(* as $output)`, map[string][]string{})
	})
	t.Run("test no index argument", func(t *testing.T) {
		check(t, `b(,,,,* as $output)`, map[string][]string{})
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
				a.get();
				print(a.get());
			}
		}
		`,
			`print(* #-> * as $target)`,
			map[string][]string{
				"target": {"12"},
			},
			ssaapi.WithLanguage(ssaconfig.JAVA),
		)
	})
}

func Test_Function_Parameter(t *testing.T) {
	code := `
	f = (i) => {
		print(i)
	}
	f(12)
	`

	t.Run("simple test", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`f(* as $i, )`,
			map[string][]string{
				"i": {"12"},
			},
		)
	})

	t.Run("simple test top def", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`f(* #-> * as $i, )`,
			map[string][]string{
				"i": {"12"},
			},
		)
	})

	t.Run("simple test bottom user", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`f(* --> * as $i, )`,
			map[string][]string{
				"i": {"FreeValue-print(Parameter-i)"},
			},
		)
	})

	t.Run("only get call", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, code,
			`f() as $call`,
			map[string][]string{
				"call": {"Function-f(12)"},
			},
		)
	})
}

func Test_Function_Parameter_Call(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (i) => {
			i()
		}
		f(()=>{print(1)})
		`,

			`f(*() as $i, )`,
			map[string][]string{
				"i": {"Parameter-i()"},
			},
		)
	})
}

func Test_ArgumentAndRest(t *testing.T) {
	t.Run("test argument and call ", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `f(1, 2, 3)`,
			`f(* as $a, ,* as $c) as $call`,
			map[string][]string{
				"a":    {"1"},
				"c":    {"3"},
				"call": {"Undefined-f(1,2,3)"},
			},
		)
	})

	t.Run("test argument and function", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `
		f = (i) => {
			i()
		}
		f(()=>{print(1)})
		`, `f(*() as $i) as $fun`, map[string][]string{
			"i":   {"Parameter-i()"},
			"fun": {"Function-f(Function-@main$1)"},
		})
	})
}
