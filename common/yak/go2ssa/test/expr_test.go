package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestExpr_normol(t *testing.T) {
	t.Run("add sub mul div", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

		func main(){
		 	var a = 10.0
			var b = 5.0
			
			var add = a + b
			var sub = a - b
			var mul = a * b
			var div = a / b
			
			println(add)
			println(sub)
			println(mul)
			println(div)
		}
		`, []string{"15","5","50","2"}, t)
	})

	t.Run("float", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

		func main(){
		 	var a = 10.0
			var b = 4.0 + a 
			var c = b / a

			
			println(a)
			println(b)
			println(c)
		}
		`, []string{"10","14","1.4"}, t)
	})

	t.Run("assign add", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

		func main(){
		 	var a = 0
			a++
			var b = 1
			b += a

			println(a)
			println(b)
		}
		`, []string{"1","2"}, t)
	})
}