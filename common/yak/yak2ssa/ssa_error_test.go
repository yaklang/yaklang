package yak2ssa

import (
	"errors"
	"reflect"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
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
			ssa4analyze.ValueUndefined("i"),
			ssa4analyze.ValueUndefined("j"),
			ssa4analyze.ValueUndefined("a"),
			ssa4analyze.ValueUndefined("b"),
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
			ssa4analyze.ValueUndefined("a2"),
		},
	})
}

func TestBasicExpression(t *testing.T) {
	t.Run("basic assign", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
				a = 1
				b = a

				a1 := 1
				b = a1
				`,
		})
	})

	t.Run("test type variable", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			typeof(1) == map[int]string
			`,
			ExternInstance: map[string]any{
				"typeof": reflect.TypeOf,
			},
		})
	})

	t.Run("undefined lexical", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			a == undefined
			`,
			errs: []string{
				ssa4analyze.ValueUndefined("a"),
			},
		})
	})
}

func TestFreeValue(t *testing.T) {
	t.Run("freeValue ahead ExternInstance", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			param() // extern value 
			param = "" // value
			delayFuzz =() =>{
				param.a().b() // freeValue 
			}
			`,
			ExternInstance: map[string]any{
				"param": func() {},
			},
		})
	})

	t.Run("freeValue force assign in block", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			{
				a  := 1
				f = () => {
					b := a
				}
			}

			{
				a := 1
				if 1 {
					b := 2
					f = () => {
						c = b // get b(2) FreeValue
					}
				}
			}
			`,
		})
	})
}

func TestPhi(t *testing.T) {
	t.Run("test phi ", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			for 1 {
				b = str.F()
			}
			b = 2

			for 2 {
				str.F2() // only handler "field str[F2]" 
			}
			`,
			ExternInstance: map[string]any{
				"str": map[string]any{
					"F":  func() int { return 1 },
					"F2": func() {},
				},
			},
		})
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
			ssa4analyze.ValueUndefined("b"),
			ssa4analyze.ValueUndefined("param"),
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

		a = [1, 2, 3]
		func3(a...) // this pass
		`,
			errs: []string{
				ssa4analyze.NotEnoughArgument("func1", "", "number"),
				ssa4analyze.NotEnoughArgument("func2", "number", "number, number"),
				ssa4analyze.NotEnoughArgument("func2", "", "number, number"),
				ssa4analyze.NotEnoughArgument("func3", "", "number, ...number"),
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
				ssa4analyze.CallAssignmentMismatch(3, 2),
				ssa4analyze.CallAssignmentMismatch(2, 3),
			},

			ExternInstance: map[string]any{
				"func1": func() int { return 1 },
				"func2": func() (a, b int) { return 1, 2 },
				"func3": func() (a, b, c int) { return 1, 2, 3 },
			},
		})
	})
}

// for  "check alias type method"
type AliasType int

func (a AliasType) GetInt() int {
	return int(a)
}

// for "check extern type recursive"
type AStruct struct {
	A []AStruct
	B BStruct
}
type BStruct struct {
	A *AStruct
}

func TestExternStruct(t *testing.T) {
	t.Run("check alias type method", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			b = getAliasType()
			b.GetInt()
			b.GetInt()
			b.GetInt()
			`,
			ExternInstance: map[string]any{
				"getAliasType": func() AliasType { return AliasType(1) },
			},
		})
	})

	//TODO: handle type recursive
	t.Run("check extern type recursive", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			a = getA()
			`,
			ExternInstance: map[string]any{
				"getA": func() *AStruct { return &AStruct{} },
			},
		})
	})
}

func TestErrorHandler(t *testing.T) {
	t.Run("error handler check", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: ` 
		// this ok
		getError1()
		getError2()

		err = getError1() 
		die(err)
		a, err = getError2()
		if err {
			panic(err)
		}

		// not handle error
		err = getError1()     // error 
		a, err = getError2()  // error

		// (1) = (n contain error) 
		all = getError2()
		`,
			errs: []string{
				ssa4analyze.ErrorUnhandled(),
				ssa4analyze.ErrorUnhandled(),
			},
			ExternInstance: map[string]any{
				"getError1": func() error { return errors.New("err") },
				"getError2": func() (int, error) { return 1, errors.New("err") },
				"die":       func(error) {},
			},
		})
	})

	t.Run("recover error", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			err := recover()
			if err != nil {
				print(err.Error())
			}
			`,
			ExternInstance: map[string]any{
				"print": func(any) {},
			},
		})
	})

}

func TestTryCatch(t *testing.T) {
	t.Run("try catch cfg", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			try {
				a = 1
				a1 = 1
			} catch err {
				a = 2
				// a1 = 2 // a1 undefine
			}
			b = a
			b = a1

			try {
				a2 = 1
				a3 = 1
			} catch err {
				a2 = 2
				a3 = 2
			} finally {
				a2 = 3
				// a3 = 3 // a3 undefine
			}
			b = a2
			b = a3
			`,
			errs: []string{
				ssa4analyze.ValueUndefined("a1"),
				ssa4analyze.ValueUndefined("a3"),
			},
		})
	})

}
