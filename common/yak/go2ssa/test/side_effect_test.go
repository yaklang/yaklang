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
				a := 3
				println(a) // 3
				f1()	   // f1产生的side-effect(2,a)与'a:=1'绑定,不会影响到'a:=3'
				println(a) // 3
			}
			println(a) // 1
			f2()
			println(a) // side-effect(2, a)
		}


		`, []string{"3", "3", "1", "side-effect(2, a)"}, t)
	})

	t.Run("side-effect nesting bind", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`package main

	func main() {
		a := 1
		f1 := func() {
			a = 10
		}
		{
			a := 2
			f2 := func() {
				a = 20
			}
			f3 := func() {
			    f1()
			}

			f2() 
			println(a) // 20 a2: 2->20
		}
	}
		`, []string{
			"side-effect(20, a)",
		}, t)
	})

	t.Run("side-effect nesting bind2", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`package main

	func main() {
		a := 1
		f1 := func() {
			a = 10
		}
		{
			a := 2
			f2 := func() {
				a = 20
			}
			f3 := func() {
			    f1()
			}

			f3()
			println(a) // 2 a1: 1->10
		}
	}
		`, []string{
			"2",
		}, t)
	})

	t.Run("side-effect cross block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`package main

	func main() {
		a := 1
		f1 := func() {
			a = 10
		}
		{
			f2 := func() {
				a = 20
			}
			f3 := func() {
			    f1()
			}

			f3()
			println(a) // 20 a1: 1->10
		}
	}
		`, []string{
			"side-effect(20, a)",
		}, t)
	})
}
