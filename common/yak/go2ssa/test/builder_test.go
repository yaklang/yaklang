package test

import (
	"testing"

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

func TestTemp(t *testing.T) {
	t.Run("temp", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

		func b() int{
			return 3
		}

		func main(){
			a := 6
		 	if a = 1; a > 1 {
		 	}else{
				a = 7
		 	}
		 	println(a)
		}

		`, []string{
			"3",
		}, t)
	})
}		
