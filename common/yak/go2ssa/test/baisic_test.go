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
			println(a)
		    a = 6
			println(a)
			return
		}

		func main(){
			a := test()
		}

		`, []string{
			"0", "6",
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
	t.Run("baisic", func(t *testing.T) {
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

func TestType_normol(t *testing.T) {
	t.Run("baisic unassign", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

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
			
		`, []string{"0", "\"\"", "false", "0"}, t)
	})

	t.Run("baisic", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

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
			
		`, []string{"1", "\"hello\"", "true", "100.5"}, t)
	})

	t.Run("multi-line string", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
		func main(){
			println(`+"`"+`hello
world`+"`"+`)
		}`,
			[]string{"\"hello\\nworld\""}, t)
	})

	t.Run("slice array", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		func main(){
			var a [3]int = [3]int{1, 2, 3}
			var b []int = []int{1, 2, 3}
			var c []string = []string{"1", "2", "3"}

			println(a)
			println(b[1])
			println(c)
		}
			
		`, []string{"make([]number)", "2", "make([]string)"}, t)
	})

	t.Run("map", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		func main(){
			var a map[int]string = map[int]string{1:"1", 2:"2", 3:"3"}
			var b map[string]int = map[string]int{"1":1, "2":2, "3":3}
			var c map[string]string = map[string]string{"1":"1", "2":"2", "3":"3"}

			println(a[1])
			println(b["1"])
			println(c)
		}
			
		`, []string{"\"1\"", "1", "make(map[string]string)"}, t)
	})

	t.Run("chan", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		func main(){
			ch1 := make(chan int)
			ch2 := make(chan string)

			println(ch1)
			println(ch2)
		}
			
		`, []string{"Function-make(typeValue(chan number))", "Function-make(typeValue(chan string))"}, t)
	})

	t.Run("struct", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type mystruct struct{
		    a int 
			b string
			c []int
		}

		func main(){
			t := mystruct{a:1,b:"hello",c:[]int{1,2,3}}
			println(t.a)
			println(t.b)
			println(t.c[2])
		}
			
		`, []string{"1", "\"hello\"", "3"}, t)
	})

	t.Run("closure", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func newCounter() func() int {
			count := 1
			return func() int {
				count++
				return count
			}
		}

		func main(){
			counter := newCounter()
			println(counter())
		}
		`, []string{
			"Function-newCounter()() binding[1]",
		}, t)
	})

	t.Run("interface", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
		type s struct {
			a, b int
		}

		type i interface {
			Add() int
			Sub() int
		}

		func (i *s) Add() int {
			return i.a + i.b
		}

		func (i *s) Sub() int {
			return i.a - i.b
		}

		func do(i i) {
			println(i.Add())
			println(i.Sub())
		}

		func main(){
			b := &s{a: 3, b: 3}
			do(b)
		}
		`, []string{
			"Undefined-i.Add(valid)(Parameter-i)", "Undefined-i.Sub(valid)(Parameter-i)",
		}, t)
	})
}

func TestType_struct(t *testing.T) {
	t.Run("struct inheritance", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		type A struct {
			a int 
			b int 
		}
	
		type B struct {
			A
			a int
			c int
		}
	
		func main (){
			a := A{a: 1, b: 2 }
			b := B{A: a, a: 3, c: 4}
			println(a.a) // 1
			println(b.a) // 3
			println(b.b) // 2
		}
		`, []string{"1", "3", "Undefined-b.b(valid)"}, t)
	})

}

func TestType_nesting(t *testing.T) {
	t.Run("map slice nesting", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
		    str := map[string][]string{
				"baidu.com":{"http://baidu.com/asdasd","https://baidu.com"},
			}
			println(str["baidu.com"][0])
			println(str["baidu.com"][1])
		}

		`, []string{"\"http://baidu.com/asdasd\"", "\"https://baidu.com\""}, t)
	})

	t.Run("struct map nesting", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		type Typ struct {
			a []string
			b map[string]string
		}

		func main(){
			typ := Typ{
				a: []string{"1","2","3"},
				b: map[string]string{
					"baidu.com": "http://baidu.com",
				},
			}
			println(typ.a[0])
			println(typ.b["baidu.com"])
		}

		`, []string{"\"1\"", "\"http://baidu.com\""}, t)
	})

	t.Run("slice struct nesting", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		type Typ struct {
			a int
			b string
		}

		func main(){
			slice := []Typ{Typ{1,"a"},Typ{b:"b"}}
			println(slice[0].a)
			println(slice[0].b)
			println(slice[1].a)
			println(slice[1].b)
		}

		`, []string{"1", "\"a\"", "0", "\"b\""}, t)
	})
}
