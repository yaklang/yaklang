package test

import (
	"fmt"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestMemberCall(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = {}
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	t.Run("normal slice", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = [] 
		a[0] = 1
		println(a[0])
		`, []string{"1"}, t)
	})

}

func TestMemberCallNegative(t *testing.T) {

	/// check v
	t.Run("expr is undefine, create before", func(t *testing.T) {
		test.CheckPrintlnValue(`
		b = a
		println(a.b)
		`, []string{"Undefined-b.b(valid)"}, t)
	})

	t.Run("expr is undefine, create right-now", func(t *testing.T) {
		test.CheckPrintlnValue(`
		println(a.b)
		`, []string{"Undefined-a.b(valid)"}, t)
	})

	t.Run("expr conn't be index", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a.b)
		`, []string{
			"Undefined-a.b",
		}, t)
	})

	// in left
	t.Run("expr is undefine in left", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})
	t.Run("expr is undefine, create before, in left", func(t *testing.T) {
		test.CheckPrintlnValue(`
		b = a
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	t.Run("expr is, conn't be index, in left", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	// expr = {}
	t.Run("expr is make", func(t *testing.T) {
		test.CheckPrintlnValue(`
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
		test.CheckPrintlnValue(`
		v = {}
		println(v.key)
		`, []string{
			"Undefined-v.key(valid)",
		}, t)
	})

	t.Run("expr normal, key is type", func(t *testing.T) {
		test.CheckPrintlnValue(`
		v = "111"
		println(v[1])
		`, []string{
			"Undefined-v.1(valid)",
		}, t,
		)
	})

}

func TestMemberCall_Negative_Closure(t *testing.T) {
	t.Run("create in this function", func(t *testing.T) {
		test.CheckPrintlnValue(`
		f = () => {
			v = {"a": 1}
			println(v.a)
		}
		`, []string{"1"}, t)
	})

	t.Run("create undefine in this function", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
			f = () =>{
				v = {} 
				println(v.a)
			}
			`,
			Want: []string{
				"Undefined-v.a(valid)",
			},
		})
	})

}

func TestMemberCall_CheckField(t *testing.T) {

	t.Run("assign in same scope", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = {} 
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	t.Run("assign in same scope printf undefine", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = {} 
		println(a.b)
		a.b = 1
		println(a.b)
		`, []string{"Undefined-a.b(valid)", "1"}, t)
	})

	t.Run("assign", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = {} 
		if c {
			a.b = 1
		}
		println(a.b)
		`, []string{
			"phi(a.b)[1,Undefined-a.b(valid)]",
		}, t)
	})

	t.Run("read", func(t *testing.T) {
		test.CheckTestCase(t, test.TestCase{
			Code: `
		a = {}
		if c {
			println(a.b)
		}
		println(a.b)
			`,
			Check: func(p *ssaapi.Program, want []string) {
				printlns := p.Ref("println").ShowWithSource()
				arg := printlns.GetUsers().Filter(func(v *ssaapi.Value) bool {
					return v.IsCall()
				}).Flat(func(v *ssaapi.Value) ssaapi.Values {
					return ssaapi.Values{v.GetOperand(1)}
				}).ShowWithSource()

				argUniqed := lo.UniqBy(arg, func(v *ssaapi.Value) int64 {
					return v.GetId()
				})

				require.Len(t, argUniqed, 1)
			},
		})
	})
}

func TestMemberCall_Struct(t *testing.T) {
	type A struct {
		AField int
	}

	check := func(t *testing.T, code string, want []string) {
		test.CheckPrintf(t, test.TestCase{
			ExternValue: map[string]any{
				"GetA": func() A { return A{} },
			},
			Code: code,
			Want: want,
		})
	}

	t.Run("normal", func(t *testing.T) {
		check(t, `
		a = GetA()
		println(a.AField)
		`, []string{
			"Undefined-a.AField(valid)",
		})
	})

	t.Run("invalid", func(t *testing.T) {
		check(t, `
		a = GetA()
		println(a.UUUUUUU)
		`, []string{
			"Undefined-a.UUUUUUU",
		})
	})
}
func TestMemberCall_Method(t *testing.T) {
	check := func(t *testing.T, code string, want []string) {
		test.CheckPrintf(t, test.TestCase{
			Code: code,
			Want: want,
			ExternValue: map[string]any{
				"getExample": func() test.ExampleInterface { return test.ExampleStruct{} },
			},
		})
	}

	t.Run("normal", func(t *testing.T) {
		check(t, `
		a = getExample()
		println(a.ExampleMethod())
		`, []string{
			"Undefined-a.ExampleMethod(valid)(Function-getExample())",
		})
	})
}

func Test_CallMember_Method(t *testing.T) {
	t.Run("test method call", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = getExample()
			a.ExampleMethod()
			`,
			ExternValue: map[string]any{
				"getExample": test.GetExampleInterface,
			},
		})
	})

	t.Run("test fieldFunction call", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
			a = getExample()
			a.ExampleFieldFunction()
			`,
			ExternValue: map[string]any{
				"getExample": test.GetExampleStruct,
			},
		})
	})

}

func Test_CallMember_Cfg(t *testing.T) {
	t.Run("test if", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a=()=>({}); 
		a.b = 1; 
		if e {
			a.b=3
		}; 
		d = a.b
		println(d)
		`, []string{
			"phi(d)[3,1]",
		}, t)
	})

	t.Run("test loop variable", func(t *testing.T) {
		code := `
		a = 0 
		for i=0; i<10; i++ {
			a = 1
		}
		println(a)
		`
		test.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			result, err := prog.SyntaxFlowWithError(
				`
				println as $ppp 
				println(* as $a ) as $call 
				$a #-> as $p 
				println(* #-> * as $param)
				`,
				ssaapi.QueryWithEnableDebug(),
			)
			result.Show(sfvm.WithShowAll(true))
			require.NoError(t, err)
			values := result.GetValues("param")
			fmt.Println(values.String())
			require.Contains(t, values.String(), "1")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("test loop", func(t *testing.T) {
		code := `
		a = {} 
		for i=0; i<10; i++ {
			a.b = 1
		}
		println(a.b)
		`
		test.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			result, err := prog.SyntaxFlowWithError(
				`println(* #-> * as $param)`,
				ssaapi.QueryWithEnableDebug(),
			)
			require.NoError(t, err)
			values := result.GetValues("param")
			fmt.Println(values.String())
			require.Contains(t, values.String(), "1")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}

func Test_CallMember_Make(t *testing.T) {
	t.Run("test make self variable", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = "this string" 
		a = {
			"key": a,
		}
		println(a.key)
		`, []string{
			`"this string"`,
		}, t)
	})

	t.Run("test make self method", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = "this string"
		a = {
			"get": () => {
				println(a)
				return a
			}
		}
		`, []string{"Parameter-a"}, t)
	})
	t.Run("check free value", func(t *testing.T) {
		code := `
var a= 1
for(x=1;;){
    var b = func(){
		return a
	}()
	println(b)
}`
		test.CheckSyntaxFlow(t, code,
			`println(* #-> * as $param)`,
			map[string][]string{
				"param": {"1"},
			},
			ssaapi.WithLanguage(ssaconfig.Yak),
		)
	})
	t.Run("check extern value", func(t *testing.T) {
		code := `
var a = ssa
b =  a.Yak
println(b)
`
		test.CheckSyntaxFlow(t, code, `
		println(* #-> * as $param)
		`, map[string][]string{
			"param": {"Undefined-a"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})

	t.Run("check free value1", func(t *testing.T) {
		code := `
var a = ssa
for(x=1;;){
    var b = func(){
		return a.Yak
	}()
	println(b)
}`
		test.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"Undefined-a"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test freeValue in loop", func(t *testing.T) {
		code := `var a = ssa
for(){
    if(c){
		println(a.Yak)
    }else{
        a = 1
    }
}`
		test.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{
			"param": {"Undefined-a", "1", "Undefined-c"},
		}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
	t.Run("test paramMember in loop", func(t *testing.T) {
		code := `func(a){
for{
    if(c){
		a = 1
    }else{
        println(a)
    }
}
}`
		test.CheckSyntaxFlow(t, code, `println(* #-> * as $param)`, map[string][]string{"param": {"1", "FreeValue-c", "Parameter-a"}}, ssaapi.WithLanguage(ssaconfig.Yak))
	})
}
