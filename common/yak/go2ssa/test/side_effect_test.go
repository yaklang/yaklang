package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_SideEffect(t *testing.T) {
	t.Run("side-effect bind", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
	func main(){
		a := 1
		f1 := func() {
			a = 2
		}
		f2 := func() {
			a = 3	 
		}
		println(a) // 1
		f2()
		println(a) // side-effect(3, a)
	}
		`, []string{"1", "side-effect(3, a)"}, t)
	})

	t.Run("side-effect nesting bind", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
	func main(){
		a := 1
		f1 := func() {
			a = 2
		}
		f2 := func() { 
			f1()
			println(a) // side-effect(2, a)
		}
		println(a) // 1
		f2()
		println(a) // side-effect(side-effect(2, a), a)
	}
		`, []string{"side-effect(2, a)", "1", "side-effect(side-effect(2, a), a)"}, t)
	})

	t.Run("side-effect nesting bind have local", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
	func main(){
		a := 1
		f1 := func() {
			a = 2
		}
		f2 := func() {
			a := 3	 
			f1()
			println(a) // 3
		}
		println(a) // 1
		f2()
		println(a) // side-effect(2, a)
	}
		`, []string{"3", "1", "side-effect(2, a)"}, t)
	})

	t.Run("side-effect cross block bind", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`package main

	func main(){
		a := 1
		f1 := func() {
			a = 2
		}
		{
			a := 3
			println(a) // 3
			f1()
			println(a) // 3
		}
	}
		`, []string{
			"3", "3",
		}, t)
	})

	t.Run("side-effect cross block nesting bind", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`package main

	func main(){
		a := 1
		f1 := func() {
			a = 2
		}
		{
			a := 3	 
			f2 := func() {
				f1()
				println(a) // 3
			}
			println(a) // 3
			f2()
			println(a) // 3
		}
		println(a) // side-effect(2, a)
	}
		`, []string{
			"FreeValue-a", "3", "3", "side-effect(2, a)",
		}, t)
	})

	t.Run("side-effect cross block nesting bind have local", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`package main

	func main(){
		a := 1
		f1 := func() {
			a = 2
		}
		{
			a := 3	 
			f2 := func() {
				a := 4
				f1()
				println(a) // 4
			}
			println(a) // 3
			f2()
			println(a) // 3
		}
		println(a) // side-effect(2, a)
	}
		`, []string{
			"4", "3", "3", "side-effect(2, a)",
		}, t)
	})

	t.Run("side-effect cross global", func(t *testing.T) {
		// TODO: handle global and side-effect
		t.Skip()
		ssatest.CheckPrintlnValue(`package main

	var a = 1

	func main() {
		c := func() {
			a = 2
		}
		c()
		println(a)
	}
		`, []string{
			"side-effect(2, a)",
		}, t)
	})
}

func Test_SideEffect_Bind(t *testing.T) {
	t.Run("side-effect method", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`package main

	import "fmt"

	func (t *T)setA(a int) {
	    t.a = a
	}

	type T struct {
	    a int
	}

	func test() {
		t := T{1}
		t.setA(2)

		println(t.a)// 2 会被side-effect影响
	}
		`, []string{
			"side-effect(Parameter-a, t.a)",
		}, t)
	})
}
