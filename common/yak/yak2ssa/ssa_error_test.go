package yak2ssa

import (
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type TestCase struct {
	code           string
	errs           []string
	ExternInstance map[string]any
}

func CheckTestCase(t *testing.T, tc TestCase) {
	opts := make([]Option, 0)
	if tc.ExternInstance != nil {
		opts = append(opts, WithSymbolTable(tc.ExternInstance))
	}
	prog := ParseSSA(tc.code, opts...)
	// prog.Show()
	// fmt.Println(prog.GetErrors().String())
	errs := lo.Map(prog.GetErrors(), func(e *ssa.SSAError, _ int) string { return e.Message })
	if len(errs) != len(tc.errs) {
		t.Fatalf("error len not match %d vs %d", len(errs), len(tc.errs))
	}
	for i := 0; i < len(errs); i++ {
		for errs[i] != tc.errs[i] {
			t.Fatalf("error not match %s vs %s", errs[i], tc.errs[i])
		}
	}
}

func TestCfgEmptyBasic(t *testing.T) {
	CheckTestCase(t, TestCase{
		code: `
			for i {
				if j {
					return a  
				}else {
					return b 
				}
				// unreachable
			}
			`,
		errs: []string{
			"this value undefine:i",
			"this value undefine:j",
			"this value undefine:a",
			"this value undefine:b",
		},
	})

}

func TestOnlyDeclareVariable(t *testing.T) {
	CheckTestCase(t, TestCase{
		code: `
			var a1 
			if 1 {
				a1 = 1
			}
			b = a1

			// var a2 -> undefine
			if 1 {
				a2 = 1
			}
			c = a2
			`,
		errs: []string{
			"this value undefine:a2",
		},
	})
}

func TestUndefinedLexical(t *testing.T) {
	CheckTestCase(t, TestCase{
		code: `
			a == undefined
			`,
		errs: []string{
			"this value undefine:a",
		},
	})
}

func TestFreeValueAheadExternInstance(t *testing.T) {
	CheckTestCase(t, TestCase{
		code: `
			param() // extern value 
			param = "" // value
			delayFuzz =() =>{
				param.a().b() // freeValue 
			}
			`,
		errs: []string{},
		ExternInstance: map[string]any{
			"param": func() {},
		},
	})

}

func TestMemberCall(t *testing.T) {
	CheckTestCase(t, TestCase{
		code: `
		b.F()
		param.F() // param is extern variable
		`,
		errs: []string{
			"member call target Error",
			"this value undefine:b",
			"this value undefine:param",
		},
		ExternInstance: map[string]any{
			"param": func() {},
		},
	})
}

func TestCallParamReturn(t *testing.T) {
	// check argument
	t.Run("check argument", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: ` 
		func1(1)
		func1() // err

		func2(1, 2)
		func2(1)
		func2()

		func3(1, 2, 3)
		func3(1, 2)
		func3(1)
		func3()
		`,
			errs: []string{
				"not enough arguments in call func1 have ([]) want (number)",
				"not enough arguments in call func2 have ([1]) want (number, number)",
				"not enough arguments in call func2 have ([]) want (number, number)",
				"not enough arguments in call func3 have ([]) want (number, ...number)",
			},
			ExternInstance: map[string]any{
				"func1": func(a int) {},
				"func2": func(a, b int) {},
				"func3": func(a int, b ...int) {},
			},
		})
	})

	t.Run("check return", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			// just call
			// (0) = (n)
			func1()
			func2()
			func3()

			// (n) = (n) 
			a = func1()
			a, b = func2()
			a, b, c = func3()

			// (1) = (n) 
			a = func2()
			a = func3()

			// (m) = (n) 
			// m != 1 && m != n
			a, b, c = func2() // get error 3 vs 2
			a, b = func3()    // get error 2 vs 3
			`,
			errs: []string{
				"function call assignment mismatch: left: 3 variable but right return 2 values",
				"function call assignment mismatch: left: 2 variable but right return 3 values",
			},

			ExternInstance: map[string]any{
				"func1": func() int { return 1 },
				"func2": func() (a, b int) { return 1, 2 },
				"func3": func() (a, b, c int) { return 1, 2, 3 },
			},
		})
	})
}

