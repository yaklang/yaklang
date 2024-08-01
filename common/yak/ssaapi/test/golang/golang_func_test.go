package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)


func Test_Function_Parameter(t *testing.T) {
	t.Run("multiple parameter first", func(t *testing.T) {
		code := `package main

			func f(a,b int) int {
				return a
			}

			func main(){
				c := f(1,2)
			}
		`
		ssatest.Check(t, code,
			ssatest.CheckTopDef_Equal("c", []string{"1"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("multiple parameter second", func(t *testing.T) {
		code := `package main

			func f(a,b int) int {
				return b
			}

			func main(){
				c := f(1,2)
			}
		`
		ssatest.Check(t, code,
			ssatest.CheckTopDef_Equal("c", []string{"2"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}


func Test_Function_Return(t *testing.T) {
	t.Run("multiple return first", func(t *testing.T) {
		ssatest.Check(t, `package main
		func c() (int,int) {
			return 1,2
		}

		func main(){
			a,b:=c()
		}
		`,
			ssatest.CheckTopDef_Equal("a", []string{"1"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("multiple return second", func(t *testing.T) {
		ssatest.Check(t, `package main
		func c() (int,int) {
			return 1,2
		}

		func main(){
			a,b:=c()
		}
		`,
			ssatest.CheckTopDef_Equal("b", []string{"2"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("defaut return", func(t *testing.T) {
		ssatest.Check(t, `package main

		func test()(a int){
		    a = 6
			return
		}

		func main(){
			r := test()
		}
		`,
			ssatest.CheckTopDef_Equal("r", []string{"6"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

/*
func Test_Function_Global(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.Check(t, `package main
		var a = 1

		func main(){
		}
		`, ssatest.CheckTopDef_Equal("a", []string{"1"}),
		ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}*/

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
		ssatest.Check(t, code,
			ssatest.CheckTopDef_Equal("b", []string{"1"}),
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
			num1 := a
			c()
			num2 := a
		}
		`
		ssatest.Check(t, code,
			ssatest.CheckTopDef_Equal("num1", []string{"1"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("side-effect after", func(t *testing.T) {
		code := `package main

		func main(){
			a := 1
			c := func (){
				a = 2
			}
			num1 := a
			c()
			num2 := a
		}
		`
		ssatest.Check(t, code,
			ssatest.CheckTopDef_Equal("num2", []string{"2"}),
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

