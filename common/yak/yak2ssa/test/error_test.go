package test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

func TestUndefineError(t *testing.T) {
	t.Run("cfg empty basicBlock", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			for i {
				if j {
					return a
				}else {
					return b
				}
				// unreachable
			}
			`,
			Want: []string{
				ssa.ValueUndefined("i"),
				ssa.ValueUndefined("j"),
				ssa.ValueUndefined("a"),
				ssa.ValueUndefined("b"),
			},
		})
	})

	t.Run("undefined field function", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = c
			b = c
			a = undefinePkg.undefineField
			a = undefinePkg.undefineFunc(a);
			b = undefineFunc2("bb")
			for i=0; i<10; i++ {
				undefineFuncInLoop(i)
			}
			`,
			Want: []string{
				ssa.ValueUndefined("c"),
				ssa.ValueUndefined("c"),
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
		test.CheckError(t, test.TestCase{
			Code: `
			a = f"${undefined_var}"
			`,
			Want: []string{
				ssa.ValueUndefined("undefined_var"),
			},
		})
	})

	t.Run("undefine in closure", func(t *testing.T) {
		code := `
			a = () => {
				b = xxx
			}
			a()
			`
		test.CheckError(t, test.TestCase{
			Code: code,
			Want: []string{
				ssa.BindingNotFound("xxx", memedit.NewRange(memedit.NewPosition(5, 4), memedit.NewPosition(5, 7))),
				ssa.BindingNotFoundInCall("xxx"),
			},
			ExternValue: map[string]any{},
			ExternLib:   map[string]map[string]any{},
		})
	})
}

func TestErrorComment(t *testing.T) {
	t.Run("test basic undefine error", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
			Want: []string{
				ssa.ValueUndefined("c"),
				ssa.ValueUndefined("d"),
				ssa.ValueUndefined("e"),
				ssa.ValueUndefined("f"),
				ssa.NoCheckMustInFirst(),
			},
		})
	})

	t.Run("test nocheck in first line ", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `// @ssa-nocheck
			print(a)
			print(b)
			print(c)
			`,
		})
	})
}

func TestBasicExpression(t *testing.T) {
	t.Run("only declare variable ", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
			Want: []string{
				ssa.ValueUndefined("a2"),
				ssa.ValueUndefined("a2"),
				ssa4analyze.ConditionIsConst("if"),
				ssa4analyze.ConditionIsConst("if"),
			},
		})
	})

	t.Run("test type variable", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			typeof(1) == map[int]string
			`,
			ExternValue: map[string]any{
				"typeof": reflect.TypeOf,
			},
		})
	})

	t.Run("undefined lexical", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a == undefined
			`,
			Want: []string{
				ssa.ValueUndefined("a"),
			},
		})
	})
}

func TestAssign(t *testing.T) {
	t.Run("value assignment 1=1", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			// 1 = 1
			a = 1
			`,
		})
	})
	t.Run("value assignment 1=n", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			// 1 = n
			a = 1, 2
			a = 1, 2, 3
			`,
		})
	})

	t.Run("value assignment n=1", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			// 1 = n
			n = 1, 2
			a, b = n
			`,
		})
	})

	t.Run("value assignment n=n", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			// n = n
			a, b, c = 1, 2, 3
			`,
		})
	})
	t.Run("value assignment m=n", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			// m = n
			a, b = 1, 2, 3       // err 2 != 3
			a, b, c = 1, 2, 3, 4 // err 3 != 4
			`,
			Want: []string{
				yak2ssa.MultipleAssignFailed(2, 3),
				yak2ssa.MultipleAssignFailed(3, 4),
			},
		})
	})
}

func TestFreeValue(t *testing.T) {
	t.Run("freeValue ahead ExternInstance", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			param("a") // extern value
			param = "" // value
			f =() =>{
				b = param[1]
			}
			`,
			Want: []string{
				ssa.ContAssignExtern("param"),
			},
		})
	})

	t.Run("freeValue force assign in block", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
			Want: []string{
				ssa4analyze.ConditionIsConst("if"),
			},
		})
	})

	t.Run("free-value capture value", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			f = () => {b = 1}
			{
				a = 1
				f = () => {
					b = a
				}
			}
			f()
			`,
		})
	})
}

func TestPhi(t *testing.T) {
	t.Run("test phi ", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			for 1 {
				b = lib.F()
			}
			b = 2

			for 2 {
				lib.F2() // only handler "field str[F2]"
			}

			for 3 {
				lib.F()
				lib.F = 1
				lib.F2()
				lib.F()
			}
			`,
			Want: []string{
				ssa.ContAssignExtern("lib.F"),
			},
			ExternLib: map[string]map[string]any{
				"lib": {
					"F":  func() int { return 1 },
					"F2": func() {},
				},
			},
		})
	})
}

func TestForEach(t *testing.T) {
	t.Run("for in variadic parameter", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			f = (first...) => {
				for num in first {
				}
			}
			`,
			Want: []string{
				"empty block",
			},
		})
	})

	t.Run("for in with chan", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			ch = make(chan int)

			for i in ch { // ok
				_ = i
			}

			for i, v in ch { // error
				_ = i
			}
			`,
			Want: []string{
				yak2ssa.InvalidChanType("chan number"),
			},
		})
	})
}

func TestErrorMemberCall(t *testing.T) {
	t.Run("normal member call", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
		test.CheckError(t, test.TestCase{
			Code: `
			param("").String() // param is extern variable
			`,
			Want: []string{},
		})
	})

	t.Run("undefined variable member call", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			b.E = 1
			`,
			Want: []string{
				ssa.ValueUndefined("b"),
			},
		})
	})

	// TODO: handle this case in type check rule
	t.Run("unable member call", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
			Want: []string{
				ssa.InvalidField("number", "F"),
				ssa.InvalidField("number", "B"),
				ssa.InvalidField("() -> number", "B"),
				ssa.InvalidField("number", "B"),
				// ssa.InvalidField("number", "$Key"),
			},
		})
	})

	t.Run("left member call", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = {
				"A": 1,
			}

			a["A"] = 2
			a.A = 3

			Key = "A"
			a.$Key = 4
			a.$UndefineKey = 5 // this err in yakast
			`,
			Want: []string{
				ssa.ValueUndefined("UndefineKey"),
			},
		})
	})

	t.Run("any value member call", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			f = (a) => {
				b = a.Field
				c = b.Field
			}
			`,
		})
	})
	t.Run("test map type ", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = {"a": 1, 2: 2}
			b = ["1", 2, "3"]
			`,
		})
	})

	t.Run("test the extern key of phi for member call 1 ", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `  
  isPost := cli.Bool("isPost")
  cli.check()
  Location = "AppendHTTPPacketQueryParam"
  if isPost {
    Location = "appendHeade"
  }
  poc[Location]`,
			Want: []string{
				ssa.ExternFieldError("Lib", "poc", "appendHeade", "appendHeader"),
				ssa.ExternFieldError("Lib", "poc", "appendHeade", "appendHeader"),
				ssa.InvalidField("any", "Location"),
			},
		})
	})

	t.Run("test the extern key of phi for member call 2", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `  
  isPost := cli.Bool("isPost")
  cli.check()
  Location = "AppendHTTPPacketQueryParam"
  if isPost {
    Location = "appendHeader"
  }
  poc[Location]`,
			Want: []string{},
		})
	})

	t.Run("test the extern key of phi for member call in for stmt", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `  
  isPost := cli.Bool("isPost")
  cli.check()
  Location = "AppendHTTPPacketQueryParam"
  for i := 0; i < 10; i++ {
     poc[Location]
  }
 `,
			Want: []string{},
		})
	})

	t.Run("test the extern key of phi for member call in for stmt extend", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `  
	ispost = cli.Bool("ispost")
	cli.check()
	Location = "AppendHTTPPacketQueryParam"
	
	for true {
		if ispost{
			Location = "AppendHTTPPacketPostParam"
		}
		packetRaw = poc[Location]
	}
 `,
			Want: []string{},
		})
	})

	t.Run("test recursive extern phi key", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `  
	ispost = cli.Bool("ispost")
	cli.check()
	Location = "AppendHTTPPacketQueryParam"
	
	if ispost{
		if ispost{
			Location = "AppendHTTPPacketPostParam"
		}
	}
	packetRaw = poc[Location]
 `,
			Want: []string{},
		})
	})

	t.Run("test extern key: for stmt + if stmt", func(t *testing.T) {
		t.Skip("Location in if stmt should be replaced")
		test.CheckError(t, test.TestCase{
			Code: `  
ispost = cli.Bool("ispost")
cli.check()
Location = "AppendHTTPPacketQueryParam"
for true {
    if ispost {
         poc[Location]
    }
    packetRaw = poc[Location]
}

 `,
			Want: []string{},
		})
	})
}

func TestSliceCall(t *testing.T) {
	t.Run("normal slice call", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = [1, 2, 3]
			a[1] = 1
			a[2] = 3
			`,
		})
	})

	t.Run("unable slice call", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
			Want: []string{
				ssa.ValueUndefined("a1"),
				ssa.ValueUndefined("a1"),
				ssa.InvalidField("number", "1"),
				ssa.InvalidField("number", "1"),
			},
			ExternValue: map[string]any{
				"print": func(any) {},
			},
		})
	})

	t.Run("slice call with string type", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = "abc"
			a[1] = 1
			a[2] = 3
			`,
			Want: []string{},
		})
	})

	t.Run("slice call with number type left ", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = 1 
			a[1] = 1
			`,
			Want: []string{
				ssa.InvalidField("number", "1"),
			},
		})
	})

	t.Run("slice call with number type right ", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = 1 
			b = a[1]
			`,
			Want: []string{
				ssa.InvalidField("number", "1"),
			},
		})
	})

	t.Run("any value slice call", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			f = (a) => {
				b = a[0]
				c = b[0]
			}
			`,
		})
	})

	t.Run("slice type check", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = make([]int, 0)
			b = a[0]
			b = a.CCCCC
			`,
			Want: []string{
				ssa.ExternFieldError("Type", "[]number", "CCCCC", "Cap"),
			},
		})
	})
}

func TestType(t *testing.T) {
	t.Run("check bytes", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			f1([]byte{1, 2})
			f1([]uint8{1, 2})
			f1([]int{1, 2})
			f1([]byte([]int{1, 2}))
			f1(string("aaaa"))
			`,
			Want: []string{
				ssa4analyze.ArgumentTypeError(1, "[]number", "bytes", "f1"),
			},
			ExternValue: map[string]any{
				"f1": func([]byte) {},
			},
		})
	})

	t.Run("check nil", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			fint(nil)
			fstring(nil)
			fbytes(nil)
			`,
			Want: []string{
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

	t.Run("check ortype", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `  
ispost = cli.Bool("ispost")
cli.check()
Location = "AppendHTTPPacketQueryParam"

if ispost {
	Location = 9999
}
Location.HasPrefix("/")

 `,
			Want: []string{},
		})
	})
}

func TestCallParamReturn(t *testing.T) {
	// check argument
	t.Run("check argument", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
			Want: []string{
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

	t.Run("check return, all right", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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

			`,
			ExternValue: map[string]any{
				"func1": func() int { return 1 },
				"func2": func() (a, b int) { return 1, 2 },
				"func3": func() (a, b, c int) { return 1, 2, 3 },
			},
		})
	})

	t.Run("check return, (2) = (1)", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			// (m) = (n)
			// m != 1 && m != n
			a, b = func1()    // get error 2 vs 1
			`,
			Want: []string{
				ssa.CallAssignmentMismatch(2, "number"),
				ssa.ValueUndefined("b"),
			},

			ExternValue: map[string]any{
				"func1": func() int { return 1 },
			},
		})
	})

	t.Run("check return, (3) = (2)", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `

			// (m) = (n)
			// m != 1 && m != n
			a, b, c = func2() // get error 3 vs 2
			`,
			Want: []string{
				ssa.CallAssignmentMismatch(3, "number, number"),
				ssa.ValueUndefined("c"),
			},

			ExternValue: map[string]any{
				"func2": func() (a, b int) { return 1, 2 },
			},
		})
	})
	t.Run("check return, (2) = (3)", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `

			// (m) = (n)
			// m != 1 && m != n
			a, b = func3()    // get error 2 vs 3
			`,
			Want: []string{
				ssa.CallAssignmentMismatch(2, "number, number, number"),
			},

			ExternValue: map[string]any{
				"func3": func() (a, b, c int) { return 1, 2, 3 },
			},
		})
	})

	t.Run("check return field", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
			Want: []string{
				ssa.InvalidField("number, number", "2"),
				ssa.CallAssignmentMismatch(2, "number"),
				ssa.ValueUndefined("b"),
				ssa.InvalidField("number", "0"),
				ssa.InvalidField("number", "1"),
			},
			ExternValue: map[string]any{
				"func1": func() int { return 1 },
				"func2": func() (a, b int) { return 1, 2 },
				"func3": func() (a, b, c int) { return 1, 2, 3 },
			},
		})
	})

	t.Run("test function parameter", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
		test.CheckError(t, test.TestCase{
			Code: `
			f1 = ()=> {
				go f2()
			}
			f2 = () => {
			}
			`,
			Want: []string{
				"empty block",
				ssa4analyze.FreeValueUndefine("f2"),
			},
		})
	})
}

func TestClosureBinding(t *testing.T) {
	t.Run("use free value", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
			Want: []string{
				// ssa.BindingNotFound("a2",memedit.NewRange(memedit.NewPosition(0, 18, 3), memedit.NewPosition(0, 18, 7), "")),
				// ssa.BindingNotFoundInCall("a2"),
			},
		})
	})

	t.Run("use free value with instance function", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			fn {
				b = a1
			}

			f = () => {
				b = a2
			}
			f()
			`,
			Want: []string{
				ssa.BindingNotFound("a1", memedit.NewRange(memedit.NewPosition(2, 4), memedit.NewPosition(4, 5))),
				ssa.BindingNotFound("a2", memedit.NewRange(memedit.NewPosition(9, 4), memedit.NewPosition(9, 7))),
				ssa.BindingNotFoundInCall("a2"),
			},
		})
	})

	t.Run("use free-value in loop-if", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
			Want: []string{
				"empty block",
			},
		})
	})

	t.Run("use parameter value", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
		test.CheckError(t, test.TestCase{
			Code: `
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

	t.Run("modify parameter value", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
		test.CheckError(t, test.TestCase{
			Code: `
			f = () => {
				a = 1
			}
			f()
			a = 2
			`,
		})
	})

	t.Run("function factor", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
		test.CheckError(t, test.TestCase{
			Code: `
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
		test.CheckError(t, test.TestCase{
			Code: `
			a = getA()
			`,
			ExternValue: map[string]any{
				"getA": func() *AStruct { return &AStruct{} },
			},
		})
	})

	t.Run("check extern type in next-loop", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
		test.CheckError(t, test.TestCase{
			Code: `
			a = getCanGetInt()
			a.GetInt()
			`,
			ExternValue: map[string]any{
				"getCanGetInt": func() CanGetInt { return AliasType(1) },
			},
		})
	})

	t.Run("extern type field error", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = getA()
			b = a.C
			print(b)
			a.GetA()
			`,
			Want: []string{
				ssa.ExternFieldError("Type", "test.AStruct", "GetA", "GetAStruct"),
				ssa.InvalidField("test.AStruct", "C"),
			},
			ExternValue: map[string]any{
				"getA":  func() *AStruct { return &AStruct{} },
				"print": func(...any) {},
			},
		})
	})

	t.Run("extern function return type map", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
		test.CheckError(t, test.TestCase{
			Code: `
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
		test.CheckError(t, test.TestCase{
			Code: `
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
			Want: []string{
				ssa.ExternFieldError("Lib", "lib", "GetInt", "getInt"),
				ssa.InvalidField("any", "GetInt"),
				ssa.ExternFieldError("Lib", "lib", "GetaInt", "getAInt"),
				ssa.InvalidField("any", "GetaInt"),
				ssa.ContAssignExtern("lib.getInt"),
				// ssa.ContAssignExtern("lib.GetInt"),
				ssa.ContAssignExtern("lib"),
				ssa.ContAssignExtern("print"),
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
		test.CheckError(t, test.TestCase{
			Code: `
			fun1("1")
			fun("1")
			`,
			Want: []string{
				ssa4analyze.NotEnoughArgument("fun", "string", "bytes, boolean"),
			},
			ExternValue: map[string]any{
				"fun":  func([]byte, bool) {},
				"fun1": func(...byte) {},
			},
		})
	})

	t.Run("use extern instance free value", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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
		test.CheckError(t, test.TestCase{
			Code: `
			// this ok
			getError1()
			`,
			Want:        []string{},
			ExternValue: map[string]any{"getError1": func() error { return errors.New("err") }},
		})
	})

	t.Run("error handler check - getError2", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			// this ok
			getError2()
			`,
			Want:        []string{},
			ExternValue: map[string]any{"getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})

	t.Run("error handler check - die", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			err = getError1()
			die(err)
			`,
			Want:        []string{},
			ExternValue: map[string]any{"getError1": func() error { return errors.New("err") }, "die": func(error) {}},
		})
	})
	t.Run("error handler check - if", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a, err = getError2()
			if err {
				panic("error ")
			}
			`,
			Want:        []string{},
			ExternValue: map[string]any{"getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})
	t.Run("error handler check - panic", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a, err = getError2()
			if err {
				panic(err)
			}
			`,
			Want:        []string{},
			ExternValue: map[string]any{"getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})

	t.Run("ignore handle error", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			_ = getError1()     // error
			a, _ = getError2()  // error
			`,
			Want:        []string{},
			ExternValue: map[string]any{"getError1": func() error { return errors.New("err") }, "getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})

	t.Run("not handle error", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			err = getError1()     // error
			a, err = getError2()  // error
			`,
			Want: []string{
				ssa4analyze.ErrorUnhandled(),
				ssa4analyze.ErrorUnhandled(),
			},
			ExternValue: map[string]any{"getError1": func() error { return errors.New("err") }, "getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})

	t.Run("error handler check - all", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			all = getError2() // this has error !!
			all2 = getError2()
			all2[1] // err
			`,
			Want: []string{
				ssa4analyze.ErrorUnhandledWithType("number, error"),
				ssa4analyze.ErrorUnhandled(),
			},
			ExternValue: map[string]any{"getError2": func() (int, error) { return 1, errors.New("err") }},
		})
	})

	t.Run("function error with drop, return  null", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = getError1()~
			`,
			Want: []string{
				ssa.ValueIsNull(),
			},
			ExternValue: map[string]any{
				"getError1": func() error { return errors.New("err") },
			},
		})
	})

	t.Run("function error with drop, return int", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = getError2()~
			a, err = getError2()~
			`,
			Want: []string{
				ssa.CallAssignmentMismatchDropError(2, "number"),
				ssa.ValueUndefined("err"),
			},

			ExternValue: map[string]any{
				"getError2": func() (int, error) { return 1, errors.New("err") },
			},
		})
	})

	t.Run("function error with drop, return int, int", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = getError3()~
			a, b = getError3()~
			a, b, err = getError3()~
			`,
			Want: []string{
				ssa.ValueUndefined("err"),
				ssa.CallAssignmentMismatchDropError(3, "number, number"),
			},
			ExternValue: map[string]any{
				"getError3": func() (int, int, error) { return 1, 2, errors.New("err") },
			},
		})
	})

	t.Run("recover error", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
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

	t.Run("a function with errors was called", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			codec.EncodeToHex(codec.AESCBCDecrypt('', '', ''))
			`,
			Want: []string{
				ssa4analyze.ErrorUnhandledWithType(`bytes, error`),
			},
		})
	})

	t.Run("the function that handled the error was called", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			codec.EncodeToHex(codec.AESCBCDecrypt('', '', '')~)
			`,
			Want: []string{},
		})
	})
}

func TestTryCatch(t *testing.T) {
	t.Run("try catch cfg", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = 0 
			try {
				a = 1
				a1 = 1
			} catch err {
				a = 2
			}
			b = a 
			b = a1 // undefine

			a2 = 0 
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
			b = a3 // undefine
			`,
			Want: []string{
				ssa.ValueUndefined("a1"),
				ssa.ValueUndefined("a1"),
				ssa.ValueUndefined("a3"),
				ssa.ValueUndefined("a3"),
			},
		})
	})

	t.Run("catch block err variable ", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			try {
			} catch err {
				a = (err)
			}finally {
			}
			`,
			Want: []string{
				"empty block",
				"empty block",
			},
		})

		test.CheckError(t, test.TestCase{
			Code: `
			try{
			}catch err {
			} finally{
				a = err
			}
			`,
			Want: []string{
				"empty block",
				"empty block",
				ssa.ValueUndefined("err"),
				ssa.ValueUndefined("err"),
			},
		})
	})
}

func TestSwitch(t *testing.T) {
	t.Run("test switch cfg", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
		fruit = "banana";

		switch (fruit) {
 			case "apple":
  			case "banana":
    			a = 1
  			case "orange":
    			a = 2
  			default:
    			b = a // undefine
			}
        `,
			Want: []string{
				ssa.ValueUndefined("a"),
				ssa.ValueUndefined("a"),
				ssa4analyze.ConditionIsConst("switch"),
				// ssa4analyze.BlockUnreachable(),
			},
		})
	})

	t.Run("test fallthrough in switch ", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: ` 
			a = 1
			switch (a) {
			case 1: 
				fallthrough
			default: 
				if a == 2{
					fallthrough
				}
			}
		`,
			Want: []string{
				yak2ssa.UnexpectedFallthroughStmt(),
				ssa4analyze.ConditionIsConst("switch"),
				ssa4analyze.ConditionIsConst("if"),
			},
		})
	})
}

func TestErrorGenericFunc(t *testing.T) {
	t.Run("append-int-with-string", func(t *testing.T) {
		test.CheckError(t,
			test.TestCase{
				Code: `
a = [1]
target = append(a, 2, "3")`,
				Want: []string{
					ssa.GenericTypeError(ssa.TypeT, ssa.TypeT, ssa.CreateNumberType(), ssa.CreateStringType()),
					ssa4analyze.ArgumentTypeError(3, "string", "number", "append"),
				},
			},
		)
	})
	t.Run("append-bytes-with-bytes", func(t *testing.T) {
		test.CheckError(t,
			test.TestCase{
				Code: `
a = b"asd"
target = append(a, b"qwe", b"zxc")`,
				Want: []string{
					ssa.GenericTypeError(ssa.TypeT, ssa.TypeT, ssa.CreateByteType(), ssa.CreateBytesType()),
				},
			},
		)
	})
}

func TestParameterMember(t *testing.T) {
	t.Run("assign", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = 1
			f = (a) => {
				a.b = 1 
			}
			f(a)
			`,
			Want: []string{
				ssa.InvalidField("number", "b"),
			},
		})
	})

	t.Run("read", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = 1
			f = (a) => {
				b = a.b
			}
			f(a)
			`,
			Want: []string{
				ssa.ValueNotMember(ssa.SSAOpcodeConstInst, "a", "b", memedit.NewRange(
					memedit.NewPosition(6, 4),
					memedit.NewPosition(6, 8),
				)),
				ssa.ValueNotMemberInCall("a", "b"),
			},
		})
	})

	t.Run("++, read and write by variable", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = 1
			f = (a) => {
				a.b += 1
			}
			f(a)
			`,
			Want: []string{
				ssa.ValueNotMember(ssa.SSAOpcodeConstInst, "a", "b", memedit.NewRange(
					memedit.NewPosition(6, 4),
					memedit.NewPosition(6, 8),
				)),
				ssa.ValueNotMemberInCall("a", "b"),
				ssa.InvalidField("number", "b"),
			},
		})
	})
}
