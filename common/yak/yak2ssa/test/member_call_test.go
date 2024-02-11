package test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestMemberCall(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		checkPrintlnValue(`
		a = {}
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	t.Run("normal slice", func(t *testing.T) {
		checkPrintlnValue(`
		a = [] 
		a[0] = 1
		println(a[0])
		`, []string{"1"}, t)
	})

}

func TestMemberCallNegative(t *testing.T) {

	/// check v
	t.Run("expr is undefine, create before", func(t *testing.T) {
		checkPrintlnValue(`
		b = a
		println(a.b)
		`, []string{"Undefined-#0.b(valid)"}, t)
	})

	t.Run("expr is undefine, create right-now", func(t *testing.T) {
		checkPrintlnValue(`
		println(a.b)
		`, []string{"Undefined-#1.b(valid)"}, t)
	})

	t.Run("expr conn't be index", func(t *testing.T) {
		checkPrintlnValue(`
		a = 1
		println(a.b)
		`, []string{
			"Undefined-#0.b",
		}, t)
	})

	// in left
	t.Run("expr is undefine in left", func(t *testing.T) {
		checkPrintlnValue(`
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})
	t.Run("expr is undefine, create before, in left", func(t *testing.T) {
		checkPrintlnValue(`
		b = a
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	t.Run("expr is, conn't be index, in left", func(t *testing.T) {
		checkPrintlnValue(`
		a = 1
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	// expr = {}
	t.Run("expr is make", func(t *testing.T) {
		checkPrintlnValue(`
		a = {
			"A": 1,
		}

		println(a["A"])

		a["A"] = 2
		println(a["A"])
		`, []string{
			"1", "2",
		}, t)
	})

	// check key
	t.Run("expr normal, but undefine expr.key,", func(t *testing.T) {
		checkPrintlnValue(`
		v = {}
		println(v.key)
		`, []string{
			"Undefined-#2.key(valid)",
		}, t)
	})

	t.Run("expr normal, key is type", func(t *testing.T) {
		checkPrintlnValue(`
		v = "111"
		println(v[1])
		`, []string{
			"Undefined-#0[1](valid)",
		}, t,
		)
	})

}

func TestMemberCall_Negative_Closure(t *testing.T) {
	t.Run("create in this function", func(t *testing.T) {
		checkPrintlnValue(`
		f = () => {
			v = {"a": 1}
			println(v.a)
		}
		`, []string{"1"}, t)
	})

	t.Run("create undefine in this function", func(t *testing.T) {
		CheckPrintf(t, TestCase{
			code: `
			f = () =>{
				v = {} 
				println(v.a)
			}
			`,
			want: []string{
				"Undefined-#3.a(valid)",
			},
			ExternValue: map[string]any{
				"println": func() {},
			},
		})
	})

}

func TestMemberCall_CheckField(t *testing.T) {
	t.Run("assign", func(t *testing.T) {
		checkPrintlnValue(`
		a = {} 
		if c {
			a.b = 1
		}
		println(a.b)
		`, []string{
			"phi(#2.b)[1,Undefined-#2.b(valid)]",
		}, t)
	})

	t.Run("read", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
		a = {}
		if c {
			println(a.b)
		}
		println(a.b)
			`,
			Check: func(t *testing.T, p *ssaapi.Program, want []string) {
				test := assert.New(t)
				printlns := p.Ref("println").ShowWithSource()
				arg := printlns.GetUsers().Filter(func(v *ssaapi.Value) bool {
					return v.IsCall()
				}).Flat(func(v *ssaapi.Value) ssaapi.Values {
					return ssaapi.Values{v.GetOperand(1)}
				}).ShowWithSource()

				argUniqed := lo.UniqBy(arg, func(v *ssaapi.Value) int {
					return v.GetId()
				})

				test.Len(argUniqed, 1)
			},
		})
	})
}

func TestMemberCall_Struct(t *testing.T) {
	type A struct {
		AField int
	}

	check := func(t *testing.T, code string, want []string) {
		CheckPrintf(t, TestCase{
			ExternValue: map[string]any{
				"get": func() A { return A{} },
			},
			code: code,
			want: want,
		})
	}

	t.Run("normal", func(t *testing.T) {
		check(t, `
		a = get()
		println(a.AField)
		`, []string{
			"Undefined-#0.AField(valid)",
		})
	})

	t.Run("invalid", func(t *testing.T) {
		check(t, `
		a = get()
		println(a.UUUUUUU)
		`, []string{
			"Undefined-#0.UUUUUUU",
		})
	})
}
func TestMemberCall_Method(t *testing.T) {
	check := func(t *testing.T, code string, want []string) {
		CheckPrintf(t, TestCase{
			code: code,
			want: want,
			ExternValue: map[string]any{
				"get": func() ExampleInterface { return ExampleStruct{} },
			},
		})
	}

	t.Run("normal", func(t *testing.T) {
		check(t, `
		a = get()
		println(a.ExampleMethod)
		`, []string{
			"Undefined-#0.ExampleMethod(valid)",
		})
	})
}

func Test_CallMember_Method(t *testing.T) {

	t.Run("test method call", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = get()
			a.ExampleMethod()
			`,
			ExternValue: map[string]any{
				"get": getExampleInterface,
			},
		})
	})

	t.Run("test fieldFunction call", func(t *testing.T) {
		CheckError(t, TestCase{
			code: `
			a = get()
			a.ExampleFieldFunction()
			`,
			ExternValue: map[string]any{
				"get": getExampleStruct,
			},
		})
	})

}
