package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Cross_Function(t *testing.T) {
	t.Run("multiple parameter", func(t *testing.T) {
		code := `package main

			func f1(a,b int) int {
				return a
			}

			func f2(a,b int) int {
				return b
			}

			func main(){
				c := f1(1,2)
				d := f2(1,2)
			}
		`
		ssatest.CheckSyntaxFlow(t, code, `
		c #-> as $c_def
		d #-> as $d_def
		`, map[string][]string{
			"c_def": {"1"},
			"d_def": {"2"},
		},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("defaut return", func(t *testing.T) {
		ssatest.CheckSyntaxFlow(t, `package main

		func test()(a int){
		    a = 6
			return
		}

		func main(){
			r := test()
		}
		`, `
		r #-> as $target
		`, map[string][]string{
			"target": {"6"},
		},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func Test_Function_Global(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.Check(t, `package main
		var a = 1

		func main(){
			b := a
		}
		`, ssatest.CheckTopDef_Equal("b", []string{"1"}),
		ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func Test_Closure(t *testing.T) {
	t.Run("freevalue", func(t *testing.T) {
		code := `package main

		func main(){
			a := 1
			c := func (){
				b := a
			}
		}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`b #-> as $target`,
			map[string][]string{
				"target": {"1"},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("side-effect before", func(t *testing.T) {
		code := `package main

		func main(){
			a := 1
			c := func (){
				a = 2
			}
			show := func(i int){
				num	:= i
			}
			show(a) // 1

			c()
			show(a) // side-effect(2)
		}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`num #-> as $target`,
			map[string][]string{
				"target": {"1", "2"},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}
