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
	// TODO: pointer in phi
	t.Skip()
	t.Run("pointer side-effect", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			var a int = 1
			var b int = 2
			var p *int = &a

			f := func() {
				p = &b
			}

			*p = 3
			println(a) // 3
			println(b) // 2

			f()

			*p = 4
			println(a) // 3
			println(b) // 4
		}
			
		`, []string{"3", "2", "3", "4"}, t)
	})
}
