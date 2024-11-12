package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestStmt_normol(t *testing.T) {

	t.Run("if exp", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
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

	t.Run("if stmt;exp", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
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
		test.CheckPrintlnValue(`package main
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

	t.Run("switch stmt;exp", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
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
		test.CheckPrintlnValue(`package main
		func main(){
			var i = 0
			for i < 10 {
				i++
			}
			println(i)
		}
		`, []string{"phi(i)[0,add(i, 1)]"}, t)
	})

	t.Run("for stmt;exp;stmt", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			var a = 1
			for i := 1; i < 10; i++ {
				println(i)
			}
		}
		`, []string{"phi(i)[1,add(i, 1)]"}, t)
	})

	t.Run("for range", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			for i,d := range []int{1,2,3,4,5,6,7}{
				println(i)
				println(d)
			}
		}
		`, []string{"Undefined-i(valid)", "Undefined-d(valid)"}, t)
	})
}

func TestStmt_const(t *testing.T) {
	t.Run("const", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			const (
				a int = 4
				b int = 5
				c int = 6
			)
		 	println(a)
			println(b)
			println(c)
		}
		`, []string{"4", "5", "6"}, t)
	})

	t.Run("const default", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			const (
				a int = 4
				b int = 5
				c
			)
		 	println(a)
			println(b)
			println(c)
		}
		`, []string{"4", "5", "5"}, t)
	})

	t.Run("const default iota", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			const (
				a int = 5
				b int = iota
				c
				d
			)
		 	println(a)
			println(b)
			println(c)
			println(d)
		}
		`, []string{"5", "0", "1", "2"}, t)
	})

	t.Run("const default iota Ex", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			const (
				a int = iota
				b
				c int = iota
				d
			)
		 	println(a)
			println(b)
			println(c)
			println(d)
		}
		`, []string{"0", "1", "0", "1"}, t)
	})
}

func TestExpr_normol(t *testing.T) {
	t.Run("add sub mul div", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

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
		`, []string{"15", "5", "50", "2"}, t)
	})

	t.Run("float", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
		 	var a = 10.0
			var b = 4.0 + a 
			var c = b / a

			
			println(a)
			println(b)
			println(c)
		}
		`, []string{"10", "14", "1.4"}, t)
	})

	t.Run("assign add", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
		 	a := 1
			a++
			b := 1
			b += a

			println(a)
			println(b)
		}
		`, []string{"2", "3"}, t)
	})
}

func TestFuntion_normol(t *testing.T) {
	t.Run("call", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func add(a,b int){
			return a+b
		}

		func main(){
			var c = add(1,2)
			println(c)
		}

		`, []string{"Function-add(1,2)"}, t)
	})

	t.Run("nested call", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func add1(a,b int){
		    return a+b
		}

		func add2(a,b int){
			return add1(a,b)
		}

		func main(){
			var c = add2(1,2)
			println(c)
		}

		`, []string{"Function-add2(1,2)"}, t)
	})

	t.Run("multiple return", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func ret(a,b,c int) (int,int,int){
			return a,b,c
		}

		func main(){
			println(ret(1,2,3))
		}
		`, []string{"Function-ret(1,2,3)"}, t)
	})

	t.Run("default return", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func test()(a int){
			return
		}

		func main(){
			a := test()
			println(a)
		}

		`, []string{
			"Function-test()",
		}, t)
	})

	t.Run("make", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			mapt := make(map[string]string)
			println(mapt)
		}`, []string{"Function-make(typeValue(map[string]string))"}, t)
	})

	t.Run("memcall", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
			type test struct{
				a int
				b int
			}

			func (t* test)add() (int,int) {
				return t.a, t.b
			}

			func add(t* test) (int,int) {
				return t.a, t.b
			}

			func main(){
				a := test{a: 6, b: 7}
				println(add(a))
				println(a.add())
			}
			`, []string{"Function-add(make(struct {number,number})) member[6,7]", "Undefined-a.add(valid)(make(struct {number,number})) member[6,7]"}, t)
	})
}

func TestClosu_normol(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			key := 0
			a := func(){
			    key = 6
			}
			println(key)
			a()
			println(key)
		}

		`, []string{"0", "side-effect(6, key)"}, t)
	})
}

func TestMethod_normol(t *testing.T) {
	t.Run("method", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

	import "fmt"

	type User struct {
		Id   int
		Name string
	}

	func (u * User) SetId(id int) {
		u.Id = id
	}

	func (u * User) SetName(name string) {
		u.Name = name
	}

	func main() {
		u := &User{}
		u.SetId(1)
		u.SetName("yaklang")
		println(u.Id)
		println(u.Name)
	}

		`, []string{"side-effect(Parameter-id, u.Id)", "side-effect(Parameter-name, u.Name)"}, t)
	})
}

func TestMethod_repeat(t *testing.T) {
	t.Run("method repeat", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
		type test struct{
			a int
		}

		func (t* test)add() int {
			return t.a
		}

		func add(t* test) int {
			return t.a
		}

		func main(){
			a := test{a: 1}
			println(add(a))
			println(a.add())
		}

		`, []string{"Function-add(make(struct {number})) member[1]", "Undefined-a.add(valid)(make(struct {number})) member[1]"}, t)
	})
}
