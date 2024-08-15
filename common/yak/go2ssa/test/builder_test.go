package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBuilder(t *testing.T) {
	t.Run("builder", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
		 	println("hello world")
		}

		`, []string{
			`"hello world"`,
		}, t)
	})
}

func TestTemp2(t *testing.T) {
	t.Run("multiple parameter", func(t *testing.T) {
		t.Skip()
		code := `package main

		func main(){
			a := 1
			b := 2
			{
				b = 3
				switch a = 2; a {
				default:
					println(a)
				}
				
			}
			println(b) // 3
		}
		`
		ssatest.CheckSyntaxFlow(t, code, `
		a as $a_def
		b as $b_def
		`, map[string][]string{
			"a_def": {"2"},
			"b_def": {"3"},
		},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func TestTemp(t *testing.T) {
	t.Run("temp", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`package main

		func main(){
			i := 10
			for i := 5; i < 10; i++ {
				println(i) // phi
			}
			println(i) // 10
		}
		`, []string{
			"phi(i)[5,add(i, 1)]", "10",
		}, t)
	})
}
