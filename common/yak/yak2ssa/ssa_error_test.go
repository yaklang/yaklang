package yak2ssa

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze/pass"
	"golang.org/x/exp/slices"
)

type TestCase struct {
	code        string
	errs        []string
	ExternValue map[string]any
	ExternLib   map[string]map[string]any
}

func CheckTestCase(t *testing.T, tc TestCase) {
	opts := make([]Option, 0)
	if tc.ExternValue != nil {
		opts = append(opts, WithExternValue(tc.ExternValue))
	}
	if tc.ExternLib != nil {
		for name, table := range tc.ExternLib {
			opts = append(opts, WithExternLib(name, table))
		}
	}
	prog := ParseSSA(tc.code, opts...)
	prog.Show()
	fmt.Println(prog.GetErrors().String())
	errs := lo.Map(prog.GetErrors(), func(e *ssa.SSAError, _ int) string { return e.Message })
	slices.Sort(errs)
	slices.Sort(tc.errs)
	if len(errs) != len(tc.errs) {
		t.Fatalf("error len not match %d vs %d", len(errs), len(tc.errs))
	}
	for i := 0; i < len(errs); i++ {
		for errs[i] != tc.errs[i] {
			t.Fatalf("error not match %s vs %s", errs[i], tc.errs[i])
		}
	}
}

func TestUndefine(t *testing.T) {
	t.Run("cfg empty basicBlock", func(t *testing.T) {
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
	})

	t.Run("undefine field function", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			a = c
			b = c
			a = undefinePkg.undefineField
			a = undefinePkg.undefineFunc(a); 
			b = undefineFunc2("bb")
			for i=0; i<10; i++ {
				undefineFuncInLoop(i)
			}
			`,
			errs: []string{
				ssa4analyze.ValueUndefined("c"),
				ssa4analyze.ValueUndefined("undefinePkg"),
				ssa4analyze.ValueUndefined("undefineFunc2"),
				ssa4analyze.ValueUndefined("undefineFuncInLoop"),
			},
		})
	})

	t.Run("undefined value in template string", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			a = f"${undefine}"
			`,
			errs: []string{
				ssa4analyze.ValueUndefined("undefine"),
			},
		})
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

	t.Run("only declare variable ", func(t *testing.T) {
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
	})

	t.Run("test type variable", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			typeof(1) == map[int]string
			`,
			ExternValue: map[string]any{
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

func TestAssign(t *testing.T) {
	t.Run("multiple value assignment ", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			// 1 = 1 
			a = 1

			// 1 = n
			a = 1, 2
			a = 1, 2, 3

			// n = n
			a, b, c = 1, 2, 3

			// m = n 
			a, b = 1, 2, 3       // err 2 != 3
			a, b, c = 1, 2, 3, 4 // err 3 != 4
			`,
			errs: []string{
				MultipleAssignFailed(2, 3),
				MultipleAssignFailed(3, 4),
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
			f =() =>{
				param.a().b() // freeValue 
			}
			`,
			ExternValue: map[string]any{
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
			ExternLib: map[string]map[string]any{
				"str": {
					"F":  func() int { return 1 },
					"F2": func() {},
				},
			},
		})
	})
}

func TestMemberCall(t *testing.T) {

	t.Run("normal member call", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			a = { 
				"F": ()=>{b = 1}, 
				"F1": (a) => {b = 1}, 
				"F11": (a) => {return a},
			}
			a.F()
			a.F1(1)
			b = a.F11(1)
			`,
		})
	})

	t.Run("extern variable member call", func(t *testing.T) {
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
			ExternValue: map[string]any{
				"param": func() {},
			},
		})
	})

	// TODO: handle this case in type check rule
	t.Run("unable member call", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: ` 
			a = 1 // number 
			a.F() 
			`,
		})
	})

	t.Run("left member call", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			a = {
				"A": 1,
			}

			a["A"] = 2
			a.A = 3

			Key = "A"
			a.$Key = 4
			a.$UndefineKey = 5 // this err in yakast
			`,
			errs: []string{
				ssa4analyze.ValueUndefined("UndefineKey"),
			},
		})
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
			ExternValue: map[string]any{
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
				ssa4analyze.CallAssignmentMismatch(3, "number, number"),
				ssa4analyze.CallAssignmentMismatch(2, "number, number, number"),
			},

			ExternValue: map[string]any{
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
			ExternValue: map[string]any{
				"getAliasType": func() AliasType { return AliasType(1) },
			},
		})
	})

	t.Run("check extern type recursive", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			a = getA()
			`,
			ExternValue: map[string]any{
				"getA": func() *AStruct { return &AStruct{} },
			},
		})
	})
}

func TestExternInstance(t *testing.T) {
	t.Run("basic extern", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			a = getInt()
			b = lib.getString()
			b = lib.getString()
			for 1 {
				b = lib.getString()
			}

			// in function
			f = () => {
				a = getInt()
				b = lib.getString()
				b = lib.getString()
				for 1 {
					b = lib.getString()
				}
			}

			// in loop
			for 2 {
				a = getInt()
				b = lib.getString()
				b = lib.getString()
				for 3 {
					b = lib.getString()
				}
			}
			`,
			ExternValue: map[string]any{
				"getInt": func() int { return 1 },
			},
			ExternLib: map[string]map[string]any{
				"lib": {
					"getString": func() string { return "1" },
				},
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

		// // not handle error
		err = getError1()     // error 
		a, err = getError2()  // error

		// // (1) = (n contain error) 
		all = getError2() // this has error !!
		all2 = getError2()
		all2[1] // err 
		`,
			errs: []string{
				ssa4analyze.ErrorUnhandled(),
				ssa4analyze.ErrorUnhandled(),
				ssa4analyze.ErrorUnhandledWithType("number, error"),
				ssa4analyze.ErrorUnhandled(),
			},
			ExternValue: map[string]any{
				"getError1": func() error { return errors.New("err") },
				"getError2": func() (int, error) { return 1, errors.New("err") },
				"die":       func(error) {},
			},
		})
	})

	t.Run("function error with drop", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
			a = getError1()~
			a = getError2()~
			a, err = getError2()~
			a = getError3()~
			a, b = getError3()~
			a, b, err = getError3()~
			`,
			errs: []string{
				pass.ValueIsNull(),
				ssa4analyze.CallAssignmentMismatchDropError(2, "number"),
				ssa4analyze.CallAssignmentMismatchDropError(3, "number, number"),
			},
			ExternValue: map[string]any{
				"getError1": func() error { return errors.New("err") },
				"getError2": func() (int, error) { return 1, errors.New("err") },
				"getError3": func() (int, int, error) { return 1, 2, errors.New("err") },
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
			ExternValue: map[string]any{
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
