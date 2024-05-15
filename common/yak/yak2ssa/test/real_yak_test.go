package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
				require.Equal(t, "a.b", err.Pos.GetWordText())
			},
		})
	})
}

func Test_Real_FunctionReturn(t *testing.T) {
	t.Run("function return", func(t *testing.T) {
		ssatest.CheckNoError(t, `
		f = param =>{
			if param == 1 {
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
				ssa.BindingNotFound("b", ssa.NewRange(
					nil,
					ssa.NewPosition(10, 2),
					ssa.NewPosition(10, 9),
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
				ssa.BindingNotFound("a", ssa.NewRange(
					nil,
					ssa.NewPosition(5, 3),
					ssa.NewPosition(5, 6),
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
				ssa.BindingNotFound("a", ssa.NewRange(
					nil,
					ssa.NewPosition(5, 3),
					ssa.NewPosition(5, 6),
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
				ssa.FreeValueNotMember("a", "b", ssa.NewRange(
					nil,
					ssa.NewPosition(6, 3),
					ssa.NewPosition(6, 6),
				)),
				ssa.FreeValueNotMemberInCall("a", "b"),
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
}
