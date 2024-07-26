package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestStmt_normol(t *testing.T) {
	t.Run("if exp", func(t *testing.T) {
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

	t.Run("if exp;exp", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var a int
		 	if a = 1; a > 1 {
		 	}else{
				a = 7
		 	}
		 	println(a)
		}
		`, []string{"phi(a)[1,7]"}, t)
	})

	t.Run("switch exp", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var a int
			switch a {
				case 1:
				a = 1
				case 2:
				a = 2	
				case 3:
				a = 3
				default:
				a = 0
			}
			println(a)
		}
		`, []string{"phi(a)[1,2,3,0]"}, t)
	})

	t.Run("switch exp;exp", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var a int
			switch a=1; a {
				case 2:
				a = 2	
				case 3:
				a = 3
			}
			println(a)
		}
		`, []string{"phi(a)[2,3,1]"}, t)
	})

	t.Run("for exp", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var n = 10
			var i = 0
			for i < n {
				i++
			}
			println(i)
		}
		`, []string{"phi(i)[0,add(i, 1)]"}, t)
	})


	t.Run("for exp;exp;exp", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var a = 1
			for i := 1; i < 10; i++ {
				println(i)
			}
		}
		`, []string{"phi(i)[1,add(i, 1)]"}, t)
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
		 	a := 1
			a++
			b := 1
			b += a

			println(a)
			println(b)
		}
		`, []string{"2","3"}, t)
	})
}

func TestFuntion_normol(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

		func add(a,b int){
			return a+b
		}

		func main(){
			var c = add(1,2)
			println(c)
		}

		`, []string{"FreeValue-add(1,2)"}, t)
	})

	t.Run("nested call", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

		func add1(a,b int){
		    return add2(a,b)+add2(a,b)
		}

		func add2(a,b int){
			return add3(a,b)+add3(a,b)
		}

		func add3(a,b int){
			return a+b
		}

		func main(){
			var c = add1(1,2)
			println(c)
		}

		`, []string{"FreeValue-add1(1,2) binding[FreeValue-add2]"}, t)
	})

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

	t.Run("make test", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			mapt := make(map[string]string)
			println(mapt)
		}`, []string{"Function-make(typeValue(map[string]string))"}, t)
	})
}

func TestType_normol(t *testing.T) {
	t.Run("baisic unassign", func(t *testing.T) {
	    
		test.CheckPrintlnValue( `package main

		func main(){
			var a int
			var b string
			var c bool
			var d float64

			println(a)
			println(b)
			println(c)
			println(d)
		}
			
		`, []string{"Undefined-a","Undefined-b","Undefined-c","Undefined-d"}, t)
	})

	t.Run("baisic", func(t *testing.T) {
	    
		test.CheckPrintlnValue( `package main

		func main(){
			var a int = 1
			var b string = "hello"
			var c bool = true
			var d float64 = 100.5

			println(a)
			println(b)
			println(c)
			println(d)
		}
			
		`, []string{"1","\"hello\"","true","100.5"}, t)
	})

	t.Run("slice array", func(t *testing.T) {
	    
		test.CheckPrintlnValue( `package main

		func main(){
			var a [3]int = [3]int{1, 2, 3}
			var b []int = []int{1, 2, 3}
			var c []string = []string{"1", "2", "3"}

			println(a)
			println(b[1])
			println(c)
		}
			
		`, []string{"make([]number)","2","make([]string)"}, t)
	})

	t.Run("map", func(t *testing.T) {
	    
		test.CheckPrintlnValue( `package main

		func main(){
			var a map[int]string = map[int]string{1:"1", 2:"2", 3:"3"}
			var b map[string]int = map[string]int{"1":1, "2":2, "3":3}
			var c map[string]string = map[string]string{"1":"1", "2":"2", "3":"3"}

			println(a[1])
			println(b["1"])
			println(c)
		}
			
		`, []string{"\"1\"","1","make(map[string]string)"}, t)
	})

	t.Run("chan", func(t *testing.T) {
	    
		test.CheckPrintlnValue( `package main

		func main(){
			ch1 := make(chan int)
			ch2 := make(chan string)

			println(ch1)
			println(ch2)
		}
			
		`, []string{"Function-make(typeValue(chan number))", "Function-make(typeValue(chan string))"}, t)
	})

	t.Run("struct", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

		type mystruct struct{
		    a int 
			b string
		}

		func main(){
			t := mystruct{a:1,b:"hello",c:[]int{1,2,3}}
			println(t.a)
			println(t.b)
			println(t.c[2])
		}
			
		`, []string{"1","\"hello\"","3"}, t)
	})
}
