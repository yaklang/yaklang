package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBlock_Value(t *testing.T) {

	t.Run("if stmt;exp", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
		 	if a := 2; a > 1 {
				println(a) // 2
				a = 3
		 	}else{
				println(a) // 2
				a = 4
		 	}
		 	println(a) // Undefined-a
		}
		`, []string{
			"2","2","Undefined-a",
		}, t)
	})

	t.Run("if stmt;exp EX", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			a := 1
		 	if a := 2; a > 1 {
				println(a) // 2
				a = 3
		 	}else{
				println(a) // 2
				a = 4
		 	}
		 	println(a) // 1
		}
		`, []string{
			"2","2","1",
		}, t)
	})

	// TODO
	/*t.Run("switch stmt;exp", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		
		func main(){
			switch a := 2; a {
			default:
				println(a) // 2
				a = 3
			}
			println(a) // Undefined-a
		}
		`, []string{"2","Undefined-a"}, t)
	})*/

	t.Run("for stmt;exp;stmt", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			for i := 1; i < 10; i++ {
				println(i) // phi
			}
			println(i) // Undefined-i
		}
		`, []string{"phi(i)[1,add(i, 1)]","Undefined-i"}, t)
	})
}