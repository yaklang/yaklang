package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestFunction_normal(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

		func add(a,b int){
			return a+b
		}

		func main(){
		 	var a = 1
			var b = 2
			a += 5
			b += 5
			var c = add(a,b)
			c += 5
			
			println(c)
		}

		`, []string{"add(FreeValue-add(6,7), 5)"}, t)
	})
}


func TestFunction_return(t *testing.T) {
	t.Run("multiple return", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

		func ret(a,b int) (int,int,int){
			return a+b,a-b,a*b
		}

		func main(){
		 	var a = 1
			var b = 2
			var r1,r2,r3 = ret(a,b)
			var c = r1+r2+r3
			println(c)
		}

		`, []string{"add(add(Undefined-r1(valid), Undefined-r2(valid)), Undefined-r3(valid))"}, t)
	})
}
