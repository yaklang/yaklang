package ssaapi_test

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func Test_CrossProcess(t *testing.T) {
	t.Run("Test_CrossProcess_Analysis 1", func(t *testing.T) {
		code := `
	func A(num){
		return num
	}	

	func foo(){
		m := {"a":A(1),"b":A(2)}
		print(m)
	}
		`
		/*
			以上代码会进行两次跨过程分析，不会触发防递归机制
			m->
			  -> FreeValue-A(1)
				-> Function-A
				  -> Parameter-num
					-> 1
			  -> FreeValue-A(2)
				-> Function-A
			      -> Parameter-num
					-> 2
		*/
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule, err := prog.SyntaxFlowWithError(`print(* #-> as $res)`)
			require.NoError(t, err)
			vals := rule.GetValues("res")
			vals.Show()
			return nil
		})
	})
}

func Test_WithinProcess(t *testing.T) {
	t.Run("Test_WithinProcess_Analysis", func(t *testing.T) {
		code := `
	m = {
		"foo":r.FormValue("name"),
		"bar":template.HTML(r.FormValue("id")), 
	}
	print(m)
	`
		ssatest.Check(t, code, func(prog *ssaapi.Program) error {
			rule := `print(* #{
			hook:<<<HOOK
	*?{!opcode:call && have:'FormValue'} as $toCheck
HOOK
			}-> as $res)`
			res, err := prog.SyntaxFlowWithError(rule)
			require.NoError(t, err)
			vals := res.GetValues("res")
			vals.ShowDot()

			toCheck := res.GetValues("toCheck")
			toCheck.Show()
			effect := toCheck[0].GetEffectOn()
			require.Equal(t, effect.Len(), 3)
			require.Contains(t, effect.String(), `Undefined-r.FormValue(valid)("id")`, `Undefined-r.FormValue(valid)("name")`, "Undefined-r")

			/*
					strict digraph {
				    rankdir = "BT";
				    n1 [label="r"]
				    n10 [label="t19: #7.FormValue(t18)"]
				    n11 [label="t18: \"id\""]
				    n15 [label="t20: m.bar=#14.HTML(t19)"]
				    n18 [label="template"]

				    n2 [label="#7.FormValue"]
				    n3 [label="t11: m.foo=#7.FormValue(t10)"]
				    n4 [label="t10: \"name\""]
				    n16 [label="#14.HTML"]

				    n3 -> n4 [label=""]
				    n10 -> n11 [label=""]
				    n10 -> n2 [label=""]
				    n3 -> n4 [label=""]
				    n3 -> n2 [label=""]
				    n15 -> n16 [label=""]
				    n15 -> n10 [label=""]
				    n1 -> n2 [label=""]
				    n3 -> n2 [label=""]
				    n1 -> n2 [label=""]
				    n10 -> n11 [label=""]
				    n10 -> n2 [label=""]
				    n15 -> n16 [label=""]
				    n18 -> n16 [label=""]
				    n18 -> n16 [label=""]
				    n15 -> n10 [label=""]
				}
			*/
			return nil
		})

		t.Run("test object", func(t *testing.T) {
			code := `
	m = {
		"foo":foo(a.b),
		"bar":bar(a.b),
	}
	print(m)
`
			ssatest.Check(t, code, func(prog *ssaapi.Program) error {
				rule := `print(* #{
			hook:<<<HOOK
	*?{!opcode:call && have:'a.b'} as $toCheck
HOOK
			}-> as $res)`
				res, err := prog.SyntaxFlowWithError(rule)
				require.NoError(t, err)
				vals := res.GetValues("res")
				vals.ShowDot()

				toCheck := res.GetValues("toCheck")
				toCheck.Show()
				effect := toCheck[0].GetEffectOn()
				require.Equal(t, effect.Len(), 3)
				require.Contains(t, effect.String(), "Undefined-foo(Undefined-a.b(valid))", "Undefined-bar(Undefined-a.b(valid))", "Undefined-a")
				/*
					strict digraph {
						rankdir = "BT";
						n1 [label="foo"]
						n2 [label="t1224023: m.foo=foo(#1224020.b)"]
						n4 [label="#1224020.b"]
						n6 [label="a"]
						n9 [label="t1224028: m.bar=bar(#1224020.b)"]
						n10 [label="bar"]
						n2 -> n1 [label=""]
						n2 -> n4 [label=""]
						n9 -> n10 [label=""]
						n9 -> n4 [label=""]
						n2 -> n4 [label=""]
						n6 -> n4 [label=""]
						n6 -> n4 [label=""]
						n9 -> n10 [label=""]
						n9 -> n4 [label=""]
						n2 -> n1 [label=""]
					}
				*/
				return nil
			})
		})
	})
}
