package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_CrossFunction(t *testing.T) {
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

	t.Run("multiple return first", func(t *testing.T) {
		code := `
		package main
		func f1() (int,int) {
			return 1,2
		}

		func main(){
			c,d := f1()
		}
		`
		ssatest.CheckTopDef(t, code, "c", []string{"1"}, false,
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("multiple return second", func(t *testing.T) {
		ssatest.CheckTopDef(t, `package main

			func f1() (int,int) {
				return 1,2
			}

			func main(){
				c,d := f1()
			}
		`,
			"d", []string{"2"}, false,
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("default return", func(t *testing.T) {
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

	t.Run("multiple default return first", func(t *testing.T) {
		ssatest.CheckTopDef(t, `package main

			func f1() (a,b int) {
				a = 1
				b = 2
				return
			}

			func main(){
				c,d := f1()
			}
		`,
			"c", []string{"1"}, false,
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("multiple default return second", func(t *testing.T) {
		ssatest.CheckTopDef(t, `package main

			func f1() (a,b int) {
				a = 1
				b = 2
				return
			}

			func main(){
				c,d := f1()
			}
		`,
			"d", []string{"2"}, false,
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func Test_Function_Global(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckTopDef(t, `package main
		var a = 1

		func main(){
			b := a
		}
		`, "b", []string{"1"}, false,
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

func Test_MethodName_in_Syntaxflow(t *testing.T) {
	t.Run("syntaxflow method name", func(t *testing.T) {
		code := `package main

type T struct {
    
}

func (t *T) List() int {
    
}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`   
			List as $a
    		T_List as $b
	`,
			map[string][]string{
				"a": {"Function-T.T"},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func Test_Method_lazybuild(t *testing.T) {

	t.Run("normal", func(t *testing.T) {
		code := `package main

		type T struct {
		    a int
			b int 
		}

		func main(){
			t := T{1,2}
			c := t.add()
		}

		func (t *T) add() int{
		    return t.a + t.b + 3
		}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`c #-> as $target`,
			map[string][]string{
				"target": {"1", "2", "3"},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func Test_Function_lazybuild(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		code := `package main

		func main(){
			a := 1
			b := 2
			c := add(a,b)
		}

		func add(a,b int) int{
			return a+b+3
		}
		`
		ssatest.CheckSyntaxFlow(t, code,
			`c #-> as $target`,
			map[string][]string{
				"target": {"1", "2", "3"},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

}
