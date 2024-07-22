package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestStmt_if(t *testing.T) {
	t.Run("if exp1", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var a int
		 	if a == 1 {
		 	    a = 6
		 	}else{
				a = 7
		 	}
		 	println(a)
		}
		`, []string{"phi(a)[6,7]"}, t)
	})

	t.Run("if exp2", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var a int
		 	if a = 1; a > 1 {
		 	    a = 6
		 	}else{
				a = 7
		 	}
		 	println(a)
		}
		`, []string{"phi(a)[6,7]"}, t)
	})
}


func TestStmt_switch(t *testing.T) {
	t.Run("expr switch", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var a = 1
			switch a {
				case 1:
				a += 1
				case 2:
				a += 2	
				case 3:
				a += 3
				default:
				a = 0
			}
			println(a)
		}
		`, []string{"phi(a)[2,3,4,0]"}, t)
	})
}

func TestStmt_for(t *testing.T) {
    
	t.Run("for exp1", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var a = 1
			for i := 1; i < 10; i++ {
				println(i)
			}
		}
		`, []string{"phi(i)[1,add(i, 1)]"}, t)
	})

	t.Run("for exp2", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var n = 10
			for i := 1; i < n; i++ {
				n -= i
			}
			println(n)
			println(i)
		}
		`, []string{"phi(n)[10,sub(n, phi(i)[1,add(i, 1)])]","FreeValue-i"}, t)
	})

	
	t.Run("for exp3", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var n = 10
			var i = 0
			for i < n {
				n -= i
				i++
			}
			println(n)
			println(i)
		}
		`, []string{"phi(n)[10,sub(n, phi(i)[0,add(i, 1)])]","phi(i)[0,add(i, 1)]"}, t)
	})

	t.Run("for range", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			for i,d := range []int{1,2,3,4,5,6,7}{
				println(i)
				println(d)
			}
		}
		`, []string{"Undefined-i(valid)","Undefined-d(valid)"}, t)
	})
}
