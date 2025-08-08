package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestType_Template(t *testing.T) {
	t.Run("template type", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		type Queue[T int] struct {
			items []T
		}

		func (q *Queue[T]) Pop() T {
			item := q.items[0]
			q.items = q.items[1:]
			println(item)
		}

		`, []string{"ParameterMember-parameterMember[0].0"}, t)
	})

	t.Run("template function", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func Pop[T int | string | bool](t T) T {
			return t
		}

		func main() {

			a := Pop[int](1)
			b := Pop[string]("1")
			c := Pop[bool](true)
			println(a)
			println(b)
			println(c)
		}
		`, []string{"Function-Pop(1)", "Function-Pop(\"1\")", "Function-Pop(true)"}, t)
	})
}

func TestType_normol(t *testing.T) {
	t.Run("basic unassign", func(t *testing.T) {

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

	t.Run("basic", func(t *testing.T) {

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
			
		`, []string{"make(chan number)", "make(chan string)"}, t)
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
			"ParameterMember-parameter[0].Add(Parameter-i)", "ParameterMember-parameter[0].Sub(Parameter-i)",
		}, t)
	})

	t.Run("make", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		func main(){
			var a []int = make([]int, 10)
			var b []string = make([]string, 10)

			println(a)
			println(b)
		}
			
		`, []string{"make([]number)", "make([]string)"}, t)
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

func TestType_struct(t *testing.T) {

	// TODO: 缺少指针类型,没法识别指针
	t.Run("struct inheritance", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type A struct {
			a int
		}

		type B struct {
			A
		}

		func main() {
			a := A{}
			b := B{}

			a.a = 1
			b.a = 2
			b.A.a = 3

			println(a.a) 	// 1
			println(b.a) 	// 3
			println(b.A.a)  // 3
		}
		`, []string{"1", "3", "3"}, t)
	})

	t.Run("struct inheritance full", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type A struct {
			a int
		}

		type B struct {
			A
		}

		func main() {
			a := A{}
			b := B{a}

			a.a = 1
			b.a = 2
			b.A.a = 3

			println(a.a) 	// 1
			println(b.a) 	// 3
			println(b.A.a)  // 3
		}
		`, []string{"1", "3", "3"}, t)
	})

	t.Run("struct inheritance full extend", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type A struct {
			a int
		}

		type B struct {
			A
		}

		func main() {
			b := B{A{a: 1}}

			b.a = 2
			println(b.a) 	// 2
			b.A.a = 3

			println(b.a) 	// 3
			println(b.A.a)  // 3
		}
		`, []string{"2", "3", "3"}, t)
	})

	t.Run("struct inheritance part", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type A struct {
			a int
		}

		type B struct {
			A
		}

		func main() {
			a := A{}
			b := B{A: a}

			a.a = 1
			b.a = 2
			b.A.a = 3

			println(a.a) 	// 1
			println(b.a) 	// 3
			println(b.A.a)  // 3
		}
		`, []string{"1", "3", "3"}, t)
	})

	t.Run("struct inheritance part extend", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type A struct {
			a int
		}

		type B struct {
			A
		}

		func main() {
			b := B{A: A{a: 1}}
			println(b.a) 	// 1

			b.a = 2
			println(b.a) 	// 2

			b.A.a = 3
			println(b.a) 	// 3
		}
		`, []string{"1", "2", "3"}, t)
	})

	t.Run("struct inheritance with same name", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type A struct {
			a int
		}

		type B struct {
			a int
			A
		}

		func main() {
			a := A{}
			b := B{}

			a.a = 1
			b.a = 2
			b.A.a = 3

			println(a.a) 	// 1
			println(b.a) 	// 2
			println(b.A.a)  // 3
		}
		`, []string{"1", "2", "3"}, t)
	})

	t.Run("struct inheritance pointer", func(t *testing.T) {
		// todo
		t.Skip()
		test.CheckPrintlnValue(`package main

		type A struct {
			a int 
		}
	
		type B struct {
			*A
		}
	
		func main (){
			a := A{a: 1}
			b := B{A: &a}

			a.a = 2
			println(a.a) // 2
			b.a = 3
			println(a.a) // 3
		}

	`, []string{"2", "3"}, t)
	})

	t.Run("struct inheritance pointer extend", func(t *testing.T) {
		// todo
		t.Skip()
		test.CheckPrintlnValue(`package main

		type A struct {
			a int 
		}
	
		type B struct {
			a int
			*A
		}
	
		func main (){
			a := A{a: 1}
			b := B{A: &a, a: 2}

			a.a = 3
			println(a.a) 	// 3
			println(b.a)	// 2
			println(b.A.a)  // 3

			b.a = 4
			println(a.a) 	// 3
			println(b.a) 	// 4
			println(b.A.a)  // 3

			b.A.a = 5
			println(a.a) 	// 5
			println(b.a) 	// 4
			println(b.A.a)  // 5
		}
	`, []string{"3", "2", "3", "3", "4", "3", "5", "4", "5"}, t)
	})
}

func TestType_interface(t *testing.T) {
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
			"ParameterMember-parameter[0].Add(Parameter-i)", "ParameterMember-parameter[0].Sub(Parameter-i)",
		}, t)
	})

	t.Run("interface inherit", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
		type s struct {
			a, b int
		}
		
		type i interface {
			Add() int
			Sub() int
		}
		
		type i2 interface {
			i
			Mul() int
			Div() int
		}
		
		func (i *s) Add() int {
			return i.a + i.b
		}
		
		func (i *s) Sub() int {
			return i.a - i.b
		}
		
		func (i2 *s) Div() int {
			return i2.a / i2.b
		}
		
		func (i2 *s) Mul() int {
			return i2.a * i2.b
		}
		
		func do(i i2) {
			println(i.Add())
			println(i.Sub())
			println(i.Mul())
			println(i.Div())
		}
		
		func main() {
			b := &s{a: 3, b: 3}
			do(b)
		}
		
		`, []string{
			"ParameterMember-parameter[0].Add(Parameter-i)", "ParameterMember-parameter[0].Sub(Parameter-i)",
			"ParameterMember-parameter[0].Mul(Parameter-i)", "ParameterMember-parameter[0].Div(Parameter-i)",
		}, t)
	})

	t.Run("interface cover", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		import (
			"strconv"
		)

		type IA interface {
			Get(key string) int
		}

		type B1 struct {
			mp map[string]int
		}

		type B2 struct {
			arr []int
		}

		func (b *B1) Get(key string) int {
			return b.mp[key]
		}

		func (b *B2) Get(key string) int {
			num, _ := strconv.Atoi(key)
			return b.arr[int(num)]
		}

		var (
			_ IA = (*B1)(nil)
			_ IA = (*B2)(nil)
		)

		func main() {
			b1 := &B1{mp: map[string]int{
				"a": 1,
				"b": 2,
			}}
			b2 := &B2{
				arr: []int{1, 2, 3},
			}
			println(b1.Get("a"))
			println(b2.Get("2"))
		}

		`, []string{"Undefined-b1.Get(valid)(make(struct {map[string]number}),\"a\") member[make(map[string]number)]",
			"Undefined-b2.Get(valid)(make(struct {[]number}),\"2\") member[make([]number),Undefined-.arr.int(num)(valid)]"}, t)
	})
}

func TestType_alias(t *testing.T) {
	t.Run("alias type", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

	type a int
	type b string
	type c bool

	func main() {
		var a1 a
		var b1 b
		var c1 c
		println(a1) // 默认值 0
		println(b1) // 默认值 ""
		println(c1) // 默认值 false
	}
		`, []string{"0", `""`, "false"}, t)
	})
}
