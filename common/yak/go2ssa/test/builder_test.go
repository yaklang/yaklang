package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBuilder(t *testing.T) {
	t.Run("builder", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
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
			"b_def": {"22"},
		},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func TestTemp(t *testing.T) {
	t.Run("temp", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

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
		`, []string{
			"2","3",
		}, t)
	})
}

