package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Pointer_normal(t *testing.T) {
	t.Run("basic pointer", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			b := a
			p := &b
			*p = 2
			println(a)	// 1
			println(b)	// 2
			println(p)
			println(*p)	// 2

			b = 3
			println(*p)	// 3
		}
			
		`, []string{"1", "2", "make(Pointer)", "2", "3"}, t)
	})

	t.Run("basic pointer overwrite", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		func main(){
			var a int = 1
			var b int = 2
			var p *int
	
			p = &a
			*p = 3
			p = &b
			*p = 4
			p = &a
		
			println(a)	// 3
			println(b)	// 4
			println(*p)	// 3
		}
			
		`, []string{"3", "4", "3"}, t)
	})

	t.Run("object pointer overwrite", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		type T struct {
			n *int
		}

		func main(){
			var a, b int
			a = 1
			b = 2

			o1 := T{n: &a}
			o2 := T{n: &a}

			*o1.n = 3
			println(*o1.n)  // 3
			println(*o2.n)	// 3

			o2.n = &b
			*o2.n = 4
			println(*o1.n) // 3
			println(*o2.n) // 4
		}
			
		`, []string{"3", "3", "3", "4"}, t)
	})

	t.Run("struct pointer", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		type T struct {
			n *int
		}

		func main(){
			var a, b int
			a = 1
			b = 2

			s  := T{n: &a}
			sp := &T{n: &a}

			*s.n = 3
			println(*s.n)  // 3
			println(*sp.n)	// 3

			sp.n = &b
			*sp.n = 4
			println(*s.n) // 3
			println(*sp.n) // 4
		}
			
		`, []string{"3", "3", "3", "4"}, t)
	})

	t.Run("struct pointer overwrite", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		type T struct {
			n *int
		}

		func main(){
			var a, b int
			a = 1
			b = 2

			s1 := T{n: &a}
			s2 := T{n: &a}
			sp := &s1

			*sp.n = 3
			println(*s1.n) // 3
			println(*s2.n) // 3

			sp.n = &b
			*sp.n = 4
			println(*s1.n) // 4
			println(*s2.n) // 3

			println(a) // 3
			println(b) // 4
		}
			
		`, []string{"3", "3", "4", "3", "3", "4"}, t)
	})

	t.Run("same const reused by multiple variableMemories", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type A struct {
			a int 
		}

		func main() {
			var n1 = 1
			var str = A{a: n1}

			p := &n1
			*p = 2

			println(str.a)	// 1
			println(n1)		// 2
		}

		`, []string{"1", "2"}, t)

		test.CheckPrintlnValue(`package main

		type A struct {
			a *int 
		}

		func main() {
			var n1 = 1
			var str = A{a: &n1}

			p := &n1
			*p = 2

			println(*str.a)	// 2
			println(n1)		// 2
		}

		`, []string{"2", "2"}, t)
	})

	t.Run("alias pointer", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		func add(a, b *int) int{
			return *a + *b
		}

		func main(){
			a := 1
			b := 2

			c := add(&a, &b)
			println(a)
			println(b)
			println(c)
		}
			
		`, []string{"1", "2", "Function-add(make(Pointer),make(Pointer))"}, t)
	})
}

func Test_Pointer_muti(t *testing.T) {
	t.Run("basic muti pointer", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			var a int = 1
			var p *int = &a
			var pp **int = &p

			*p = 2
			println(a) // 2
			**pp = 3
			println(a) // 3
		}
			
		`, []string{"2", "3"}, t)
	})

	t.Run("basic muti pointer overwrite", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			b := 2

			p1 := &a
			p2 := &b

			pp := &p1
			**pp = 3
			println(a) // 3
			println(b) // 2

			pp := &p2
			**pp = 4
			println(a) // 3
			println(b) // 4
		}
			
		`, []string{"3", "2", "3", "4"}, t)
	})
}

func Test_Pointer_cfg(t *testing.T) {
	t.Run("pointer cfg block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			p := &a
			{
				*p = 3
				println(a)	// 3
				println(*p)	// 3
			}
			println(a)	// 3
			println(*p)	// 3
		}

		`, []string{"3", "3", "3", "3"}, t)
	})

	t.Run("pointer cfg block local", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			p := &a
			{
				a := 2
				*p = 3
				println(a)	// 2
				println(*p)	// 3
			}
			println(a)	// 3
			println(*p)	// 3
		}

		`, []string{"2", "3", "3", "3"}, t)
	})

	t.Run("pointer cfg cross block local", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			p := &a
			{
				a := 2
				{
					a := 3
					*p = 4 
					println(a) // 3
				}
				println(a) // 2
			}
			println(*p) // 4
		}
		`, []string{"2", "3", "4"}, t)
	})

	t.Run("pointer cfg if", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
		func main(){
			a := 1
			p := &a

			if a > 0 {
				*p = 2
			} else {
				*p = 3
			}

			println(*p) // phi(p.@value)[2,3]
			println(a)	// phi(a)[2,3]
		}
			
		`, []string{"phi(a)[2,3]", "phi(p.@value)[2,3]"}, t)
	})

	t.Run("pointer cfg switch", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			var p *int
			p = &a

			switch a {
			case 1:
				*p = 2
			case 2:
				*p = 3
			}

			println(*p) // phi(p.@value)[2,3,1]
			println(a)	// phi(a)[2,3,1]
		}
			
		`, []string{"phi(p.@value)[2,3,1]", "phi(a)[2,3,1]"}, t)
	})

	t.Run("pointer cfg switch local", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			var p *int
			p = &a

			switch a {
			case 1:
				a := 2
				*p = 2
			case 2:
				a := 3
				*p = 3
			}

			println(*p) // phi(p.@value)[2,3,1]
			println(a)	// phi(a)[2,3,1]
		}
			
		`, []string{"phi(p.@value)[2,3,1]", "phi(a)[2,3,1]"}, t)
	})

	t.Run("pointer cfg if local", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			p := &a

			if a > 0 {
				a := 2
				*p = 4
			} else {
				a := 3
				*p = 5
			}

			println(*p) // phi(p.@value)[4,5]
			println(a)	// phi(a)[4,5]
		}
			
		`, []string{"phi(a)[4,5]", "phi(p.@value)[4,5]"}, t)
	})

	// Todo: 等待pr: https://github.com/yaklang/yaklang/pull/3176
	t.Skip()
	t.Run("pointer cfg for", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			var p *int

			for p = &a; ; {
				*p = 2 
			}

			println(*p) // Undefined-p.@value
			println(a)	// phi(a)[1,2]
		}
			
		`, []string{"Undefined-p.@value", "phi(a)[1,2]"}, t)
	})

	t.Run("pointer cfg for local", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			var p *int

			for p = &a; ; {
				a := 2
				*p = 2 
			}

			println(*p) // Undefined-p.@value
			println(a)	// phi(a)[1,2]
		}
			
		`, []string{"Undefined-p.@value", "phi(a)[1,2]"}, t)
	})

	t.Run("pointer cfg for address", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			b := 2
			var p *int

			for p = &a; ; {
				p = &b
			}
			*p = 3

			println(*p) // 3
			println(a)	// phi(a)[1,3]
			println(b)	// phi(b)[2,3]
		}
			
		`, []string{"3", "phi(a)[1,3]", "phi(b)[2,3]"}, t)
	})

	// WIP: cannot achieve this function
	t.Skip()
	t.Run("pointer cfg if address", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			b := 2
			var p *int
		
			if a > b {
				p = &a
			} else {
				p = &b
			}
			*p = 3

			println(*p) // 3
			println(a)	// phi(a)[1,3]
			println(b)	// phi(b)[2,3]
		}
			
		`, []string{"3", "phi(a)[1,3]", "phi(b)[2,3]"}, t)
	})

	t.Run("pointer cfg switch address", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			b := 2
			var p *int
		
			switch a {
			case 1:
				p = &a
			case 2:
				p = &b
			}
			*p = 3

			println(*p) // 3
			println(a)	// phi(a)[1,3]
			println(b)	// phi(b)[2,3]
		}
			
		`, []string{"3", "phi(a)[1,3]", "phi(b)[2,3]"}, t)
	})
}

func Test_Pointer_sideEffect(t *testing.T) {
	t.Run("pointer side-effect", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func pointer(b *int){
			*b = 2
		}

		func main(){
			a := 1

			println(a) // 1
			pointer(&a)
			println(a) // side-effect(2, a)
		}
			
		`, []string{"1", "side-effect(2, a)"}, t)
	})

	t.Run("pointer side-effect cross block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func pointer(a *int){
			*a = 2
		}

		func main(){
			a := 1
			{
				println(a) // 1
				pointer(&a)
				println(a) // side-effect(2, a)
			}
		}
			
		`, []string{"1", "side-effect(2, a)"}, t)
	})

	t.Run("pointer side-effect cross block and local", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func pointer(a *int){
			*a = 3
		}

		func main(){
            a := 1
            {
                a := 2
                println(a) // 2
                pointer(&a)
                println(a) // side-effect(3, a)
            }
            println(a) // 1
            pointer(&a)
            println(a) // side-effect(3, a)
		}
			
		`, []string{
			"2", "side-effect(3, a)",
			"1", "side-effect(3, a)"}, t)
	})

	t.Run("pointer side-effect with struct", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type Data struct {
			value int
		}

		func modifyData(d *Data) {
			d.value = 42
		}

		func main() {
			data := Data{value: 10}
			
			println(data.value) // 10
			modifyData(&data)
			println(data.value) // side-effect(42, data.value)
		}

		`, []string{"10", "side-effect(42, data.value)"}, t)
	})

	// TODO
	t.Skip()

	t.Run("pointer side-effect with nested struct", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type Inner struct {
			x int
		}

		type Outer struct {
			inner Inner
		}

		func modifyNested(outer *Outer) {
			outer.inner.x = 99
		}

		func main() {
			outer := Outer{inner: Inner{x: 5}}
			
			println(outer.inner.x) // 5
			modifyNested(&outer)
			println(outer.inner.x) // side-effect(99, outer.inner.x)
		}

		`, []string{"5", "side-effect(99, outer.inner.x)"}, t)
	})

	t.Run("pointer side-effect with slice", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func modifySlice(s []int) {
			s[0] = 100
			s[1] = 200
		}

		func main() {
			arr := []int{1, 2, 3}
			
			println(arr[0]) // 1
			println(arr[1]) // 2
			modifySlice(arr)
			println(arr[0]) // side-effect(100, arr[0])
			println(arr[1]) // side-effect(200, arr[1])
		}

		`, []string{"1", "2", "side-effect(100, arr[0])", "side-effect(200, arr[1])"}, t)
	})

	t.Run("pointer side-effect with double pointer", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func modifyThroughDoublePointer(pp **int) {
			**pp = 77
		}

		func main() {
			a := 10
			p := &a
			
			println(a) // 10
			modifyThroughDoublePointer(&p)
			println(a) // side-effect(77, a)
		}

		`, []string{"10", "side-effect(77, a)"}, t)
	})

	t.Run("pointer side-effect multiple parameters", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func modifyMultiple(a, b, c *int) {
			*a = 11
			*b = 22
			*c = 33
		}

		func main() {
			x, y, z := 1, 2, 3
			
			println(x) // 1
			println(y) // 2
			println(z) // 3
			modifyMultiple(&x, &y, &z)
			println(x) // side-effect(11, x)
			println(y) // side-effect(22, y)
			println(z) // side-effect(33, z)
		}

		`, []string{
			"1", "2", "3",
			"side-effect(11, x)",
			"side-effect(22, y)",
			"side-effect(33, z)"}, t)
	})

	t.Run("pointer side-effect with conditional", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func conditionalModify(a *int, condition int) {
			if condition > 0 {
				*a = 50
			} else {
				*a = 60
			}
		}

		func main() {
			a := 1
			
			println(a) // 1
			conditionalModify(&a, 1)
			println(a) // side-effect(phi(a)[50,60], a)
		}

		`, []string{"1", "side-effect(phi(a)[50,60], a)"}, t)
	})

	t.Run("pointer side-effect with loop", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func modifyInLoop(a *int, count int) {
			for i := 0; i < count; i++ {
				*a = *a + 1
			}
		}

		func main() {
			a := 0
			
			println(a) // 0
			modifyInLoop(&a, 3)
			println(a) // side-effect(FreeValue-a, a)
		}

		`, []string{"0", "side-effect(FreeValue-a, a)"}, t)
	})

	t.Run("pointer side-effect with map", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func modifyMap(m map[string]int) {
			m["key1"] = 100
			m["key2"] = 200
		}

		func main() {
			m := map[string]int{"key1": 1, "key2": 2}
			
			println(m["key1"]) // 1
			println(m["key2"]) // 2
			modifyMap(m)
			println(m["key1"]) // side-effect(100, m["key1"])
			println(m["key2"]) // side-effect(200, m["key2"])
		}

		`, []string{"1", "2", "side-effect(100, m[\"key1\"])", "side-effect(200, m[\"key2\"])"}, t)
	})

	t.Run("pointer side-effect with interface", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		type Modifier interface {
			Modify()
		}

		type Data struct {
			value int
		}

		func (d *Data) Modify() {
			d.value = 42
		}

		func callModify(m Modifier) {
			m.Modify()
		}

		func main() {
			data := &Data{value: 10}
			
			println(data.value) // 10
			callModify(data)
			println(data.value) // side-effect(42, data.value)
		}

		`, []string{"10", "side-effect(42, data.value)"}, t)
	})

	t.Run("pointer side-effect with channel", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func modifyThroughChannel(ch chan int) {
			ch <- 99
		}

		func main() {
			ch := make(chan int, 1)
			ch <- 1
			
			println(<-ch) // 1
			modifyThroughChannel(ch)
			println(<-ch) // side-effect(99, <-ch)
		}

		`, []string{"1", "side-effect(99, <-ch)"}, t)
	})
}
