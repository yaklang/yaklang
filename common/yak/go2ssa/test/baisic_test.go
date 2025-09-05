package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
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

func TestStmt_spin(t *testing.T) {
	t.Run("for Spin value", func(t *testing.T) {
		test.CheckPrintlnValue(`package A

	func main() {
		var a = 1
		
		for true {
			println(a)
		}
	}
		`, []string{"1"}, t)
	})

	t.Run("for Spin array", func(t *testing.T) {
		test.CheckPrintlnValue(`package A


	func main() {
		var strg = []string{
			"hello world",
		}

		for true {
			println(strg[0])
		}
	}
		`, []string{"\"hello world\""}, t)
	})

	t.Run("for Spin secondary array", func(t *testing.T) {
		test.CheckPrintlnValue(`package A

	func main() {
		var array2D [3][3]int
		array2D[0] = [3]int{1, 2, 3}
		array2D[1] = [3]int{4, 5, 6}
		array2D[2] = [3]int{7, 8, 9}

		println(array2D[0][0])
		println(array2D[1][1])
		for true {
			println(array2D[2][2])
		}
	}
		`, []string{"1", "5", "9"}, t)
	})

	t.Run("for Spin map", func(t *testing.T) {
		test.CheckPrintlnValue(`package A

	func main() {
		var mp = map[string]int{"a": 1, "b": 2, "c": 3}
		for true {
			println(mp["a"])
		}
	}
		`, []string{"1"}, t)
	})

	t.Run("for Spin struct", func(t *testing.T) {
		test.CheckPrintlnValue(`package A

	type A struct {
	    s string
	}

	func main() {
		var str = A{s: "hello world"}
		for true {
			println(str.s)
		}
	}
		`, []string{"\"hello world\""}, t)
	})

	t.Run("for Spin value assign", func(t *testing.T) {
		test.CheckPrintlnValue(`package A

	func main() {
		var a = 1
		var b = 2
		
		for true {
			a = a + b
		}
		println(a)
	}
		`, []string{"phi(a)[1,add(a, 2)]"}, t)
	})

	t.Run("for Spin array assign", func(t *testing.T) {
		test.CheckPrintlnValueContain(`package A

	func main() {
		var str = []int{1, 2, 3}

		for true {
			str[0] = str[1]
		}
		println(str[0])
	}
		`, []string{"[2,1]"}, t)
	})

	t.Run("for Spin array add assign", func(t *testing.T) {
		test.CheckPrintlnValueContain(`package A

	func main() {
		var str = []int{1, 2, 3}

		for true {
			str[0] = str[1] + str[2]
		}
		println(str[0])
	}
		`, []string{"[add(2, 3),1]"}, t)
	})

	t.Run("for Spin secondary array add assign", func(t *testing.T) {
		test.CheckPrintlnValueContain(`package A

	func main() {
		var array2D [3][3]int
		array2D[0] = [3]int{1, 2, 3}
		array2D[1] = [3]int{4, 5, 6}
		array2D[2] = [3]int{7, 8, 9}

		for true {
			array2D[2][2] = array2D[0][0] + array2D[1][1]
		}
		println(array2D[2][2])
	}
		`, []string{"[add(1, 5),9]"}, t)
	})

	t.Run("for Spin map assign", func(t *testing.T) {
		test.CheckPrintlnValueContain(`package A

	func main() {
		var mp = map[string]int{"a": 1, "b": 2, "c": 3}
		for true {
			mp["a"] = mp["b"]
		}
		println(mp["a"])
	}
		`, []string{"[2,1]"}, t)
	})

	t.Run("for Spin map add assign", func(t *testing.T) {
		test.CheckPrintlnValueContain(`package A

	func main() {
		var mp = map[string]int{"a": 1, "b": 2, "c": 3}
		for true {
			mp["a"] = mp["b"] + mp["c"]
		}
		println(mp["a"])
	}
		`, []string{"[add(2, 3),1]"}, t)
	})

	t.Run("for Spin secondary map add assign", func(t *testing.T) {
		test.CheckPrintlnValueContain(`package A
		
	func main() {
		var mp = map[string]map[string]int{
			"a": map[string]int{"a1": 1, "a2": 2, "a3": 3},
			"b": map[string]int{"b1": 4, "b2": 5, "b3": 6},
			"c": map[string]int{"c1": 7, "c2": 8, "c3": 9},
		}

		for true {
			mp["a"]["a1"] = mp["b"]["b2"] + mp["c"]["c3"]
		}
		println(mp["a"]["a1"])
	}

		`, []string{"[add(5, 9),1]"}, t)
	})

	t.Run("for Spin struct assign", func(t *testing.T) {
		test.CheckPrintlnValueContain(`package A

	type A struct {
	    a int 
		b int
		c int
	}

	func main() {
		var str = A{a: 1, b: 2, c: 3}

		for true {
			str.a = str.b
		}
		println(str.a)
	}
		`, []string{"[2,1]"}, t)
	})

	t.Run("for Spin struct add assign", func(t *testing.T) {
		test.CheckPrintlnValueContain(`package A

	type A struct {
	    a int 
		b int
		c int
	}

	func main() {
		var str = A{a: 1, b: 2, c: 3}

		for true {
			str.a = str.b + str.c
		}
		println(str.a)
	}
		`, []string{"[add(2, 3),1]"}, t)
	})

	t.Run("for Spin closu assign", func(t *testing.T) {
		test.CheckPrintlnValue(`package A

	func main() {
		f := func() int {
		    return 1
		}

		for true {
			f = func() int {
		    	return 2
			}
			println(f())
		}
		println(f())
	}
		`, []string{"Function-f()", "phi(f)[Function-f,Function-f]()"}, t)
	})

	t.Run("for Spin array global", func(t *testing.T) {
		test.CheckPrintlnValue(`package  A
	var strg = []string{
		"hello world",
	}

	func main() {
		for true {
			println(strg[0])
		}
	}
		`, []string{"\"hello world\""}, t)
	})

	t.Run("for Spin struct global", func(t *testing.T) {
		test.CheckPrintlnValue(`package A

	type A struct {
	    s string
	}

	var strg = A{s: "hello world"}
	
	func main() {
		for true {
			println(strg.s)
		}
	}
		`, []string{"\"hello world\""}, t)
	})

	t.Run("muti for Spin value", func(t *testing.T) {
		test.CheckPrintlnValue(`package A

	func main() {
		var a = 1
		for true {
			a = 2
		    for true {
				a = 3
				println(a)
			}
			println(a)
		}
		println(a)
	}
		`, []string{"3", "phi(a)[2,3]", "phi(a)[1,phi(a)[2,3]]"}, t)
	})

	// todo
	t.Run("for Spin side-effect", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`package A

func main() {
	a := 0
	f := func() func() {
		return func() {
			a = 1
		}
	}
	f2 := func(){
		a = 2
	}

	for true {
		f2()
		println(a)
	}
}
		`, []string{"side-effect(2, a)"}, t)
	})

	// todo
	t.Run("for Spin side-effect and function assignment", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`package A

func main() {
	a := 0
	f := func() func() {
		return func() {
			a = 1
		}
	}
	f2 := func(){
		a = 2
	}

	for true {
		f2 = f()
		f2()
		println(a)
	}
}
		`, []string{"side-effect(1, a)"}, t)
	})

	t.Run("for Spin memberCall", func(t *testing.T) {
		// TODO: BUG Need see IRCode, this phi and replace not correct
		t.Skip()
		test.CheckPrintlnValueContain(`package A

		type T struct {
		    a, b int
		}

		func (t* T)add() int {
			return t.a + t.b
		}

		func main() {
			t := &T{1, 2}

		    for i := 0; i < 10; i++ {
		        t.a = t.add()
				t.b = t.add()
		    }

			println(t.a)
			println(t.b)
		}
		`, []string{
			"[1,Undefined-t.add(valid)(make(struct {number,number})) member[1,2]]",
			"[2,Undefined-t.add(valid)(make(struct {number,number})) member[1,2]]",
		}, t)
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

func TestExpr_global(t *testing.T) {
	t.Run("global array", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		var arr = []string{"a", "b", "c"}
		
		func main(){
			println(arr[0])
			println(arr[1])
			println(arr[2])
		}
		`, []string{"\"a\"", "\"b\"", "\"c\""}, t)
	})

	t.Run("global map", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		var ma = map[string]int{"a": 1, "b": 2, "c": 3}
		
		func main(){
			println(ma["a"])
			println(ma["b"])
			println(ma["c"])
		}
		`, []string{"1", "2", "3"}, t)
	})

	t.Run("global struct", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type t struct {
		    a int
		    b string
			c bool
		}

		var stru = &t{a: 1, b: "hello", c: true}
		
		func main(){
			println(stru.a)
			println(stru.b)
			println(stru.c)
		}
		`, []string{"1", "\"hello\"", "true"}, t)
	})

	t.Run("global array assign add", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		var arr = []string{1, 2, 3}
		
		func main(){
			println(arr[0] + arr[1] + arr[2])
		}
		`, []string{"6"}, t)
	})

	t.Run("global map assign add", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		var ma = map[string]int{"a": 1, "b": 2, "c": 3}
		
		func main(){
			println(ma["a"] + ma["b"] + ma["c"])
		}
		`, []string{"6"}, t)
	})

	t.Run("global struct assign add", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type t struct {
		    a int
		    b int
			c int
		}

		var stru = &t{a: 1, b: 2, c: 3}
		
		func main(){
			println(stru.a + stru.b + stru.c)
		}
		`, []string{"6"}, t)
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
		}`, []string{"make(map[string]string)"}, t)
	})

	t.Run("member-call method", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
			type test struct{
				a int
				b int
			}

			func (t* test)add() (int,int) {
			}

			func main(){
				a := test{a: 6, b: 7}
				println(a.add())
			}
			`, []string{
			"Undefined-a.add(valid)(make(struct {number,number}))",
		}, t)
	})

	t.Run("member-call top-def", func(t *testing.T) {
		test.CheckSyntaxFlow(t, `package main
		
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
				a := test{a: 4, b: 5}
				println(add(a))

				a := test{a: 6, b: 7}
				println(a.add())
			}
			`, `println( * #-> as $a)`, map[string][]string{
			"a": {"4", "5", "6", "7"},
		}, ssaapi.WithLanguage(ssaapi.GO))
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

	t.Run("method check name", func(t *testing.T) {
		code := `package main

	type T struct {
		
	}

	func (t *T) F() int {
		return 1
	}

	func main() {
		t := &T{}
		a := t.F()
		b := T_F()
		
		println(b)
	}`

		test.CheckSyntaxFlow(t, code, `
			a #-> as $a
			b #-> as $b
	`, map[string][]string{
			"a": {"1"},
			"b": {"Undefined-T_F"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}

func TestMethod_repeat(t *testing.T) {

	t.Run("method should not  get like global function ", func(t *testing.T) {
		test.CheckSyntaxFlowContain(t, `package main

		type test struct{
			a int
			b int 
		}

		func (t* test)add() int {
			return t.a
		}


		func main(){
			a := test{a: 1, b: 2}
			println(add(a)) // undefine 
			println(a.add()) // method 
		}

			`, `println(* #-> as $a)`, map[string][]string{
			"a": {"1", "Undefined-add"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("method same name with global function ", func(t *testing.T) {
		test.CheckSyntaxFlow(t, `package main

	
		type test struct{
		}

		func (t* test)add() int {
			return 2
		}
		func add(t* test) int {
			return 1
		}
		

		func main(){
			a := test{}
			println(add(a))
			println(a.add())
		}

			`, `println(* #-> as $a)`, map[string][]string{
			"a": {"1", "2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("method repeat", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		
		type test struct{
			a int
		}

		func (t *test) test(){
			a := test{a: 1}
			println(a.add()) 
			// this add should build after test
			// but ReadMember(a.add) should build it function 
		}

		func (t* test)add() int {
			return t.a 
		}


		`, []string{
			"Undefined-a.add(valid)(make(struct {number})) member[1]",
		}, t)
	})
}
