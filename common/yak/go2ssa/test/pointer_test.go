package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Pointer_normal(t *testing.T) {

	t.Run("pointer overwrite", func(t *testing.T) {

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
		
			println(a)
			println(b)
			println(*p)
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

	t.Run("structure pointer (including implicit object creation)", func(t *testing.T) {

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

	// todo
	t.Run("basic muti pointer", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`package main

		func main(){
			var a int = 1
			var p *int = &a
			var pp **int = &p

			*p = 2
			println(a)
			**pp = 3
			println(a)
		}
			
		`, []string{"2", "3"}, t)
	})
}
func Test_Pointer_cfg(t *testing.T) {

}

func Test_Pointer_sideEffect(t *testing.T) {

}
