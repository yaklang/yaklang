package test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

func TestUndefineError(t *testing.T) {
	t.Run("cfg empty basicBlock", func(t *testing.T) {
		CheckError(t, TestCase{
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
			want: []string{
				ssa.ValueUndefined("i"),
				ssa.ValueUndefined("j"),
				ssa.ValueUndefined("a"),
				ssa.ValueUndefined("b"),
			},
		})
	})

	t.Run("undefined field function", func(t *testing.T) {
		CheckError(t, TestCase{
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
			want: []string{
				ssa.ValueUndefined("c"),
				ssa.ValueUndefined("c"),
				ssa.ValueUndefined("undefinePkg"),
				ssa.ValueUndefined("undefinePkg"),
				ssa.ValueUndefined("undefineFunc2"),
				ssa.ValueUndefined("undefineFuncInLoop"),
			},
		})
	})

	t.Run("undefined value in template string", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = f"${undefined_var}"
			`,
			want: []string{
				ssa.ValueUndefined("undefined_var"),
			},
		})
	})

	t.Run("undefine in closure", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = () => {
				b = xxx
			}
			a()
			`,
			want: []string{
				ssa.BindingNotFound("xxx", ssa.NewRange(ssa.NewPosition(0, 5, 3), ssa.NewPosition(0, 5, 6), "")),
				ssa.BindingNotFoundInCall("xxx"),
			},
			ExternValue: map[string]any{},
			ExternLib:   map[string]map[string]any{},
		})
	})
}

func TestErrorComment(t *testing.T) {
	t.Run("test basic undefine error", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			// @ssa-ignore
			print(a)
			// @ssa-ignore
			print(b)
			print(c) // error

			// @ssa-ignore

			print(d) // error

			// @ssa-ignore
			// this is other commend
			print(e) // err

			// @ssa-nocheck
			// not in first line; don't work
			print(f)
			`,
			want: []string{
				ssa.ValueUndefined("c"),
				ssa.ValueUndefined("print"),
				ssa.ValueUndefined("d"),
				ssa.ValueUndefined("print"),
				ssa.ValueUndefined("e"),
				ssa.ValueUndefined("print"),
				ssa.ValueUndefined("f"),
				ssa.ValueUndefined("print"),
				ssa.NoCheckMustInFirst(),
			},
		})
	})

	t.Run("test nocheck in first line ", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `// @ssa-nocheck
			print(a)
			print(b)
			print(c)
			`,
		})
	})
}

func TestBasicExpression(t *testing.T) {
	t.Run("only declare variable ", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			var a1
			if 1 {
				a1 = 1
			}
			b = a1

			// var a2 -> undefined
			if 1 {
				a2 = 1
			}
			c = a2
			`,
			want: []string{
				ssa.ValueUndefined("a2"),
				ssa4analyze.ConditionIsConst("if"),
				ssa4analyze.ConditionIsConst("if"),
			},
		})
	})

	t.Run("test type variable", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			typeof(1) == map[int]string
			`,
			ExternValue: map[string]any{
				"typeof": reflect.TypeOf,
			},
		})
	})

	t.Run("undefined lexical", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a == undefined
			`,
			want: []string{
				ssa.ValueUndefined("a"),
			},
		})
	})
}

func TestAssign(t *testing.T) {
	t.Run("multiple value assignment ", func(t *testing.T) {
		CheckError(t, TestCase{
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
			want: []string{
				yak2ssa.MultipleAssignFailed(2, 3),
				yak2ssa.MultipleAssignFailed(3, 4),
			},
		})
	})
}

func TestFreeValue(t *testing.T) {
	t.Run("freeValue ahead ExternInstance", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			param() // extern value
			param = "" // value
			f =() =>{
				param.a().b() // freeValue
			}
			`,
			want: []string{
				ssa.ContAssignExtern("param"),
			},
			ExternValue: map[string]any{
				"param": func() {},
			},
		})
	})

	t.Run("freeValue force assign in block", func(t *testing.T) {
		CheckError(t, TestCase{
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
			want: []string{
				ssa4analyze.ConditionIsConst("if"),
			},
		})
	})
}

func TestPhi(t *testing.T) {
	t.Run("test phi ", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			for 1 {
				b = str.F()
			}
			b = 2

			for 2 {
				str.F2() // only handler "field str[F2]"
			}

			for 3 {
				str.F()
				str.F = 1
				str.F2()
				str.F()
			}
			`,
			want: []string{
				ssa.ContAssignExtern("str.F"),
			},
			ExternLib: map[string]map[string]any{
				"str": {
					"F":  func() int { return 1 },
					"F2": func() {},
				},
			},
		})
	})
}

func TestForEach(t *testing.T) {
	t.Run("for in variadic parameter", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			f = (first...) => {
				for num in first {
				}
			}
			`,
			want: []string{
				"empty block",
			},
		})
	})

	t.Run("for in with chan", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			ch = make(chan int)

			for i in ch { // ok
				_ = i
			}

			for i, v in ch { // error
				_ = i
			}
			`,
			want: []string{
				ssa4analyze.InvalidChanType("number"),
			},
		})
	})
}

func TestMemberCall(t *testing.T) {
	t.Run("normal member call", func(t *testing.T) {
		CheckError(t, TestCase{
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

	t.Run("extern variable method member call", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			param().String() // param is extern variable
			`,
			want: []string{},
			ExternValue: map[string]any{
				"param": func() time.Duration { return time.Duration(1) },
			},
		})
	})

	t.Run("undefined variable member call", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			b.E = 1
			`,
			want: []string{
				ssa.ValueUndefined("b"),
			},
		})
	})

	// TODO: handle this case in type check rule
	t.Run("unable member call", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = 1 // number
			a.F()  // invalid field number
			b = a.B // invalid field number
			a.B = 1

			f = () => { return 1}
			f.B   // invalid field null
			f().B // invalid field member
			Key = f()
			a.$Key
			`,
			want: []string{
				ssa4analyze.InvalidField("number", "F"),
				ssa4analyze.InvalidField("number", "B"),
				ssa4analyze.InvalidField("( ) -> number", "B"),
				ssa4analyze.InvalidField("number", "B"),
				ssa4analyze.InvalidField("number", "$Key"),
			},
		})
	})

	t.Run("left member call", func(t *testing.T) {
		CheckError(t, TestCase{
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
			want: []string{
				ssa.ValueUndefined("UndefineKey"),
			},
		})
	})

	t.Run("any value member call", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			f = (a) => {
				b = a.Field
				c = b.Field
			}
			`,
		})
	})
	t.Run("test map type ", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = {"a": 1, 2: 2}
			b = ["1", 2, "3"]
			`,
		})
	})
}

func TestSliceCall(t *testing.T) {
	t.Run("normal slice call", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = [1, 2, 3]
			a[1] = 1
			a[2] = 3
			`,
		})
	})

	t.Run("unable slice call", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			// undefined
			a1[1] = 1 // undefined a1
			print(a1[1])

			// const
			a = 1
			a[1] = 1 // invalid field number
			print(a[1])

			// type
			f = () => {return 1}
			a = f() // number
			a[1] = 1 // invalid field number
			print(a[1])
			`,
			want: []string{
				ssa.ValueUndefined("a1"),
				ssa.ValueUndefined("a1"),
				ssa4analyze.InvalidField("number", "1"),
				ssa4analyze.InvalidField("number", "1"),
			},
			ExternValue: map[string]any{
				"print": func(any) {},
			},
		})
	})

	t.Run("slice call with string type", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = 1
			a[1] = 1
			a = "abc"
			a[1] = 1
			a[2] = 3
			`,
			want: []string{
				ssa4analyze.InvalidField("number", "1"),
			},
		})
	})

	t.Run("any value slice call", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			f = (a) => {
				b = a[0]
				c = b[0]
			}
			`,
		})
	})

	t.Run("slice type check", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = make([]int, 0)
			b = a[0]
			b = a.bb
			`,
			want: []string{
				ssa4analyze.InvalidField("[]number", "bb"),
			},
		})
	})
}

func TestType(t *testing.T) {
	t.Run("check bytes", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			f1([]byte{1, 2})
			f1([]uint8{1, 2})
			f1([]int{1, 2})
			f1([]byte([]int{1, 2}))
			f1(string("aaaa"))
			`,
			want: []string{
				ssa4analyze.ArgumentTypeError(1, "[]number", "bytes", "f1"),
			},
			ExternValue: map[string]any{
				"f1": func([]byte) {},
			},
		})
	})

	t.Run("check nil", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			fint(nil)
			fstring(nil)
			fbytes(nil)
			`,
			want: []string{
				ssa4analyze.ArgumentTypeError(1, "null", "number", "fint"),
				ssa4analyze.ArgumentTypeError(1, "null", "string", "fstring"),
			},
			ExternValue: map[string]any{
				"fint":    func(int) {},
				"fstring": func(string) {},
				"fbytes":  func([]byte) {},
			},
		})
	})
}

func TestCallParamReturn(t *testing.T) {
	// check argument
	t.Run("check argument", func(t *testing.T) {
		CheckError(t, TestCase{
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
			want: []string{
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
		CheckError(t, TestCase{
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
			a, b = func1()    // get error 2 vs 1
			a, b, c = func2() // get error 3 vs 2
			a, b = func3()    // get error 2 vs 3
			`,
			want: []string{
				ssa4analyze.CallAssignmentMismatch(2, "number"),
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

	t.Run("check return field ", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			// normal
			a = func1()
			a, b = func2()
			c = func2()
			a = c[0]
			b = c[1]
			d = c[2] // error

			a, b = func1()  // error  2 vs (number)
			c = func1()
			a = c[0] // error invalid field
			b = c[1] // error invalid field
			`,
			want: []string{
				ssa4analyze.InvalidField("number, number", "2"),
				ssa4analyze.CallAssignmentMismatch(2, "number"),
				ssa4analyze.InvalidField("number", "0"),
				ssa4analyze.InvalidField("number", "1"),
			},
			ExternValue: map[string]any{
				"func1": func() int { return 1 },
				"func2": func() (a, b int) { return 1, 2 },
				"func3": func() (a, b, c int) { return 1, 2, 3 },
			},
		})
	})

	t.Run("test function parameter", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			count = 0
			tick1s(func() {
				count++
				return count <= 5
			})
			`,
			ExternValue: map[string]any{
				"tick1s": func(func() bool) {},
			},
		})
	})

	t.Run("test go function call", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			f1 = ()=> {
				go f2()
			}
			f2 = () => {
			}
			`,
			want: []string{
				"empty block",
			},
		})
	})
}

func TestClosureBinding(t *testing.T) {
	t.Run("use free value", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
				{
					a1 = 1
					f = () => {
						b := a1
					}
					f()
				}
				// TODO: this should be undefine f
				// f()

				{
					a2 := 1
					f2 = () => {
						b := a2
					}
					f2()
				}
				// f2() // not found

				a2 = 1
				// f2()
				`,
			want: []string{
				// ssa.BindingNotFound("a2", ssa.NewRange(ssa.NewPosition(0, 18, 3), ssa.NewPosition(0, 18, 7), "")),
				// ssa.BindingNotFoundInCall("a2"),
			},
		})
	})

	t.Run("use free value with instance function", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			fn {
				b = a1
			}

			f = () => {
				b = a2
			}
			f()
			`,
			want: []string{
				ssa.BindingNotFound("a1", ssa.NewRange(ssa.NewPosition(0, 2, 3), ssa.NewPosition(0, 4, 4), "")),
				ssa.BindingNotFound("a2", ssa.NewRange(ssa.NewPosition(0, 9, 3), ssa.NewPosition(0, 9, 6), "")),
				ssa.BindingNotFoundInCall("a2"),
			},
		})
	})

	t.Run("use free-value in loop-if", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = () => {}
			b = () => {
				a()
			}
			for i in 10 {
				b()
				if b {
					b()
				}
				b()
			}
			`,
			want: []string{
				"empty block",
			},
		})
	})

	t.Run("use parameter value", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			f = (a) => {
				innerF = () => {
					print(a)
				}
				innerF()
			}
			`,
			ExternValue: map[string]any{
				"print": func(a any) {},
			},
		})
	})
	t.Run("use parameter value 2", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = f =>{
				fun = () =>{
					println(f)
				}
				fun()
			}
			`,
			ExternValue: map[string]any{
				"println": func(a any) {},
			},
		})
	})

	// TODO: more test in `ssa_var_test.go`
	t.Run("modify free value", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			{
				b = 1
				f = () => {
					b = a
				} // sideEffect: b
				a = 2
				print(b) // 1

				f()
				print(b) // b1
			}
			a = 3
			print(b) // b1
			f()
			print(b) // b2
			`,
			ExternValue: map[string]any{
				"print": func(any) {},
			},
		})
	})

	t.Run("modify parameter value", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			f = (a) => {
				innerF = () => {
					a = 1
				}
				print(a)
				innerF()
				print(a)
			}
			`,
			ExternValue: map[string]any{
				"print": func(any) {},
			},
		})
	})

	t.Run("closure side effect", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			f = () => {
				a = 1
			}
			f()
			a = 2
			`,
		})
	})

	t.Run("function factor", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			getF = func(a) {
				return func() {
					a ++
				}
			}
			f = getF(1)
			f()
			`,
		})
	})
}

// for  "check alias type method"

type CanGetInt interface {
	GetInt() int
}

var _ CanGetInt = (*AliasType)(nil)

type AliasType int

func (a AliasType) GetInt() int {
	return int(a)
}

// for "check extern type recursive"
type AStruct struct {
	A []AStruct
	B BStruct
}

func (a AStruct) GetAStruct() AStruct {
	return a
}

type BStruct struct {
	A *AStruct
}

func TestExternStruct(t *testing.T) {
	t.Run("check alias type method", func(t *testing.T) {
		CheckError(t, TestCase{
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
		CheckError(t, TestCase{
			code: `
			a = getA()
			`,
			ExternValue: map[string]any{
				"getA": func() *AStruct { return &AStruct{} },
			},
		})
	})

	t.Run("check extern type in next-loop", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = getA()
			for i, v := range a.A {
				print(i, v)
			}

			for 1 {
				for i, v in a.A {
					print(i, v)
				}
			}
			`,
			ExternValue: map[string]any{
				"getA":  func() *AStruct { return &AStruct{} },
				"print": func(...any) {},
			},
		})
	})

	t.Run("check interface type method", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = getCanGetInt()
			a.GetInt()
			`,
			ExternValue: map[string]any{
				"getCanGetInt": func() CanGetInt { return AliasType(1) },
			},
		})
	})

	t.Run("extern type field error", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = getA()
			b = a.C
			print(b)
			a.GetA()
			`,
			want: []string{
				ssa.ExternFieldError("Type", "yak2ssa.AStruct", "GetA", "GetAStruct"),
				ssa4analyze.InvalidField("yak2ssa.AStruct", "C"),
			},
			ExternValue: map[string]any{
				"getA":  func() *AStruct { return &AStruct{} },
				"print": func(...any) {},
			},
		})
	})

	t.Run("extern function return type map", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = getA()
			a = a.E
			`,
			ExternValue: map[string]any{
				"getA": func() map[string]string { return nil },
			},
		})
	})
}

func TestExternInstance(t *testing.T) {
	t.Run("basic extern", func(t *testing.T) {
		CheckError(t, TestCase{
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

	t.Run("wrong method name", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			lib.getString()
			lib.getInt()
			lib.GetInt() // error; you meant "getInt"?
			lib.GetaInt() // error; you meant "getInt"?

			lib.getInt = 1 // warn
			lib.GetInt = 1 // warn
			lib.getInt()
			lib = {"a":1} // warn
			lib.GetInt()

			print(1)
			print = func(a) {a = 1} // warn
			print(1)
			`,
			want: []string{
				ssa.ExternFieldError("Lib", "lib", "GetInt", "getInt"),
				ssa.ExternFieldError("Lib", "lib", "GetaInt", "getAInt"),
				ssa.ContAssignExtern("lib.getInt"),
				ssa.ContAssignExtern("lib.GetInt"),
				ssa.ContAssignExtern("lib"),
				ssa.ContAssignExtern("print"),
			},
			ExternValue: map[string]any{
				"print": func(any) {},
			},
			ExternLib: map[string]map[string]any{
				"lib": {
					"getString": func() string { return "1" },
					"getInt":    func() int { return 1 },
					"getAInt":   func() int { return 1 },
				},
			},
		})
	})

	t.Run("test bytes", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			fun1("1")
			fun("1")
			`,
			want: []string{
				ssa4analyze.NotEnoughArgument("fun", "string", "bytes, boolean"),
			},
			ExternValue: map[string]any{
				"fun":  func([]byte, bool) {},
				"fun1": func(...byte) {},
			},
		})
	})

	t.Run("use extern instance free value", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			p = print // print
			f = () => { // p
				p("sub")
			}

			f2 = () => { // print
				print("sub")
			}
			`,
			ExternValue: map[string]any{
				"print": func(any) {},
			},
		})
	})
}

func TestErrorHandler(t *testing.T) {
	t.Run("error handler check - getError1", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			// this ok
			getError1()
			`,
			want:        []string{},
			ExternValue: map[string]any{"getError1": func() error { return errors.New("err") }},
		})
	})

	t.Run("error handler check - getError2", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			// this ok
			getError2()
			`,
			want:        []string{},
			ExternValue: map[string]any{"getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})

	t.Run("error handler check - die", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			err = getError1()
			die(err)
			`,
			want:        []string{},
			ExternValue: map[string]any{"getError1": func() error { return errors.New("err") }, "die": func(error) {}},
		})
	})

	t.Run("error handler check - panic", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a, err = getError2()
			if err {
				panic(err)
			}
			`,
			want:        []string{},
			ExternValue: map[string]any{"getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})

	t.Run("ignore handle error", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			_ = getError1()     // error
			a, _ = getError2()  // error
			`,
			want:        []string{},
			ExternValue: map[string]any{"getError1": func() error { return errors.New("err") }, "getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})

	t.Run("not handle error", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			err = getError1()     // error
			a, err = getError2()  // error
			`,
			want: []string{
				ssa4analyze.ErrorUnhandled(),
				ssa4analyze.ErrorUnhandled(),
			},
			ExternValue: map[string]any{"getError1": func() error { return errors.New("err") }, "getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})

	t.Run("error handler check - all", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			all = getError2() // this has error !!
			all2 = getError2()
			all2[1] // err
			`,
			want: []string{
				ssa4analyze.ErrorUnhandledWithType("number, error"),
				ssa4analyze.ErrorUnhandled(),
			},
			ExternValue: map[string]any{"getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})

	t.Run("function error with drop", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = getError1()~
			a = getError2()~
			a, err = getError2()~
			a = getError3()~
			a, b = getError3()~
			a, b, err = getError3()~
			`,
			want: []string{
				ssa4analyze.ValueIsNull(),
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
		CheckError(t, TestCase{
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
		CheckError(t, TestCase{
			code: `
			try {
				a = 1
				a1 = 1
			} catch err {
				a = 2
				// a1 = 2 // a1 undefined
			}
			b = a
			b = a1

			try {
				a2 = 1
				a3 = 1
			} catch err {
				a2 = 2
				// a3 = 2 // undefine
			} finally {
				a2 = 3
			}
			b = a2
			b = a3
			`,
			want: []string{
				ssa.ValueUndefined("a1"),
				ssa.ValueUndefined("a3"),
			},
		})
	})

	t.Run("catch block err variable ", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			try {
			} catch err {
				a = (err)
			}finally {
			}
			`,
			want: []string{
				"empty block",
				"empty block",
			},
		})

		CheckError(t, TestCase{
			code: `
			try{
			}catch err {
			} finally{
				a = err
			}
			`,
			want: []string{
				"empty block",
				"empty block",
				ssa.ValueUndefined("err"),
			},
		})
	})
}

func TestSwitch(t *testing.T) {
	t.Run("test switch cfg", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
		fruit = "banana";

		switch (fruit) {
 			case "apple":
  			case "banana":
    			a = 1
  			case "orange":
    			a = 2
  			default:
    			b = a
			}
        `,
			want: []string{
				ssa.ValueUndefined("a"),
				ssa4analyze.ConditionIsConst("switch"),
				// ssa4analyze.BlockUnreachable(),
				// ssa4analyze.BlockUnreachable(),
			},
		})
	})
}
