package test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func TestYakBuildInMethod(t *testing.T) {
	t.Run("slice insert", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		a = [] 
		a.Insert(0, 1)
	`)
	})

	t.Run("slice append in loop", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		a = [] 
		for i in [1, 2]{
			a.Append(i)
		}
		`)
	})
}

func Test_RealYak_Function(t *testing.T) {
	t.Run("object", func(t *testing.T) {
		ssatest.CheckNoError(t, `
lock := sync.NewLock()

x.Foreach([1,2], func(e){
	lock.Lock()
	println(e)
	lock.Unlock()
})
		`)
	})

	t.Run("function parameter but in loop", func(t *testing.T) {
		ssatest.CheckNoError(t, `
lock := sync.NewLock()
f = (i)=>{
    lock.Lock()
    println(i)
    lock.Unlock()
}
f(1)
for true {
    f(1)
}
`)
	})

	t.Run("function parameter", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		f = () => {
			for _, i := range [1, 2,3] {
				if i==1{
					a()
				}
			}
		}

		a = () => {
			println("a")
		}
		f()
		`)
	})

	t.Run("function free-value", func(t *testing.T) {
		ssatest.CheckNoError(t, `
	
Rawfuzz = func(p, fuzzPayload) {
    p.Fuzz(fuzzPayload)
}

() => {
    p = 1
    datas = [{"a": "B"}]
    for data in datas {
        Rawfuzz(p,data.payload)
        for 1 {
            return data.payload
        }
    }
}
		`)
	})

	t.Run("function free-value not found", func(t *testing.T) {
		// should mark error inner callee function
		ssatest.CheckTestCase(t, ssatest.TestCase{
			Code: `
			f = (a ) =>{
				println(a.b)
			}
			a = 1
			f(a)
			`,
			Check: func(prog *ssaapi.Program, a []string) {
				errs := prog.GetErrors()
				require.Len(t, errs, 2)
				err := errs[0]
				require.Equal(t, ".b", err.Pos.GetText())
			},
		})
	})
}

func Test_Real_FunctionReturn(t *testing.T) {
	t.Run("function return", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		f = p =>{
			if p == 1 {
				return nil
			}
			return 1
		}

		check = scope =>{
			if len(scope) == 0{
				return
			}
			println(scope[0])
		}

		a = f(1)
		check(a)
		`)
	})
}

func Test_RealYak_Error(t *testing.T) {
	t.Run("function return error", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		encodePayload,err = codec.AESCBCEncrypt("", "", "")
		if err {
			panic("codec AES CBC Encrypt error")
		}
		`)
	})
}

func Test_RealYak_ObjectType(t *testing.T) {
	t.Run("map[string]any", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		fuzz.HTTPRequest("reqBytes")~
		exprDetails = fuzz.FuzzCalcExpr()
		result = exprDetails.result
		`)
	})

	t.Run("member call form extern function call", func(t *testing.T) {
		ssatest.CheckNoError(t, `
			rsp, req = poc.Get("123123",)~ 
		`,
		)
	})
	t.Run("member call form extern function", func(t *testing.T) {
		ssatest.CheckParse(t, `
			rsp, req = poc.Get
		`,
		)
	})
	t.Run("member call form extern function call , ignore syntax error", func(t *testing.T) {
		ssatest.CheckParse(t, `
			rsp, req = poc.Get("123123", poc.)~
		`,
			ssaapi.WithIgnoreSyntaxError(true),
		)
	})
}

func Test_RealYak_Object_Factor(t *testing.T) {
	t.Run("test pool", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		getA = func(size){
			a = {
				"f":c =>{
					return a
				},
			}
			return a
		}

		obj = getA(10)
		obj.f(1)
		`)
	})
}

func Test_RealYak_FreeValueMemberCall(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckError(t, ssatest.TestCase{
			Code: `
		a = {
			"b": 1, 
		}
		f = (p,p2) => {
			a.b = p 
			b.c = p2
		}

		f(1, 2)
		`,
			Want: []string{
				ssa.BindingNotFound("b", memedit.NewRange(
					memedit.NewPosition(10, 3),
					memedit.NewPosition(10, 10),
				)),
				ssa.BindingNotFoundInCall("b"),
			},
		},
		)
	})
}

func Test_RealYak_FreeValue_Error(t *testing.T) {
	t.Run("mark freevalue without call: no default", func(t *testing.T) {
		ssatest.CheckError(t, ssatest.TestCase{
			Code: `
		f = () => {
			b = a
		}
		`,
			Want: []string{
				ssa4analyze.FreeValueUndefine("a"),
			},
		})
	})

	t.Run("mark freevalue with call: no found ", func(t *testing.T) {
		ssatest.CheckError(t, ssatest.TestCase{
			Code: `
			f = () => {
				b = a
			}
			f()
			`,
			Want: []string{
				ssa.BindingNotFound("a", memedit.NewRange(
					memedit.NewPosition(5, 4),
					memedit.NewPosition(5, 7),
				)),
				ssa.BindingNotFoundInCall("a"),
			},
		})
	})

	t.Run("freevalue with call but found: none error", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		f = () => {
			b = a
		}
		a = 1
		f()
		`)
	})

	t.Run("freevalue with member call, without call", func(t *testing.T) {
		ssatest.CheckError(t, ssatest.TestCase{
			Code: `
		f = () => {
			b = a.b
		}
		`,
			Want: []string{
				ssa4analyze.FreeValueUndefine("a"),
			},
		})
	})

	t.Run("freevalue  with member call with call: no found ", func(t *testing.T) {
		ssatest.CheckError(t, ssatest.TestCase{
			Code: `
			f = () => {
				b = a.b
			}
			f()
			`,
			Want: []string{
				ssa.BindingNotFound("a", memedit.NewRange(
					memedit.NewPosition(5, 4),
					memedit.NewPosition(5, 7),
				)),
				ssa.BindingNotFoundInCall("a"),
			},
		})
	})

	t.Run("freevalue  with member call with call: no member ", func(t *testing.T) {
		ssatest.CheckError(t, ssatest.TestCase{
			Code: `
			f = () => {
				b = a.b
			}
			a = 1
			f()
			`,
			Want: []string{
				ssa.ValueNotMember(ssa.SSAOpcodeConstInst, "a", "b", memedit.NewRange(
					memedit.NewPosition(6, 4),
					memedit.NewPosition(6, 7),
				)),
				ssa.ValueNotMemberInCall("a", "b"),
			},
		})
	})

	t.Run("free value with member call, with call", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		f = () => {
			b = a.b
		}
		a = {
			"b": 1,
		}
		f()
		`,
		)
	})

	t.Run("first use freevalue and then define it", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		a = func(){
			b()
		}

		b = func(){
			a() 
		}
		a()
		`)
	})
}

type A struct{}

func NewA() *A {
	return &A{}
}

type B struct{}

func (a *A) A(b *B) {
}

func Test_RealYak_LazyMethod(t *testing.T) {
	t.Run("lazy", func(t *testing.T) {
		ssatest.Check(t, `
	a = NewA()
	`, func(prog *ssaapi.Program) error {
			// not test in database
			if prog.IsFromDatabase() {
				return nil
			}
			require.NotNil(t, prog.Program.GetType("test.A"))
			require.Nil(t, prog.Program.GetType("test.B"))
			return nil
		}, ssaapi.WithExternValue(map[string]any{
			"NewA": NewA,
		}))
	})
	t.Run("lazy init", func(t *testing.T) {
		ssatest.Check(t, `
	a = NewA()
b = a.A
	`, func(prog *ssaapi.Program) error {
			// not test in database
			if prog.IsFromDatabase() {
				return nil
			}
			require.NotNil(t, prog.Program.GetType("test.A"))
			require.NotNil(t, prog.Program.GetType("test.B"))
			return nil
		}, ssaapi.WithExternValue(map[string]any{
			"NewA": NewA,
		}))
	})
}

func Test_Return_phi(t *testing.T) {
	t.Run("return phi type", func(t *testing.T) {
		code := `
		encodePayload,err = codec.AESCBCEncrypt("", "", "")
		if err {
			// panic("codec AES CBC Encrypt error")
            return 
		}	
        print(encodePayload)
		`

		symbol := yaklang.New().GetFntable()
		opts := make([]ssaconfig.Option, 0)
		tmp := reflect.TypeOf(make(map[string]interface{}))
		for name, item := range symbol {
			itype := reflect.TypeOf(item)
			if itype == tmp {
				opts = append(opts, ssaapi.WithExternLib(name, item.(map[string]interface{})))
			}
		}

		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			prog.Show()
			res, err := prog.SyntaxFlowWithError(`
			print( * as $para)
			$para<typeName()> as $typeName 
			`)
			res.Show()
			require.NoError(t, err)
			typeName := res.GetValues("typeName")
			// typeName
			require.True(t, len(typeName) == 1)
			require.Equal(t, typeName[0].String(), "\"bytes\"")

			return nil
		}, opts...)

	})
}

func Test_Object_Type_Kind_Compare(t *testing.T) {
	t.Run("cli.uiSchemaField and cli.uiSchemaGroup", func(t *testing.T) {
		ssatest.CheckError(t, ssatest.TestCase{
			Code: `
cli.uiFieldGroups(
	cli.uiField("a", 1),
)`,
			Want: []string{
				ssa4analyze.ArgumentTypeError(1, "cli.uiSchemaField", "cli.uiSchemaGroup", "cli.uiFieldGroups"),
			},
		})
	})
}

func TestYakEllipsis(t *testing.T) {
	t.Run("simple yak ellipsis", func(t *testing.T) {
		ssatest.CheckNoError(t, `
f = func(a,b,c){
	println(a)
	println(b)
	println(c)
}
f([1,2,3]...)
		`)
	})
}
