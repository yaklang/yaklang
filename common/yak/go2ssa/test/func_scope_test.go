package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestFunction_Value(t *testing.T) {

	t.Run("function", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func test(){
			a := 1
			println(a)
		}
		func main(){
			test()
		}
		`, []string{
			"1",
		}, t)
	})

	t.Run("function call", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func add(a,b int) int {
			return a+b
		}

		func main(){
			println(add)
			println(add(1,2))
		}
		`, []string{
			"Function-add","Function-add(1,2)",
		}, t)
	})

	t.Run("function call latency definition", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			var c = add(1,2)
			println(c)
		}

		func add(a,b int) int {
			return a+b
		}
		`, []string{
			"Function-add(1,2)",
		}, t)
	})

	t.Run("global value", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		var count = 1

		func f(){
			count = 2
			println(count)
		}

		func main(){
			println(count)
			f()
			println(count)
		}
		`, []string{
			"2","1","1",
		}, t)
	})
}

func TestClosu_Value(t *testing.T) {
	t.Run("closu freevalue", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			f := func() {
				println(a)
			}
			println(a)
		}
		`, []string{
			"FreeValue-a","1",
		}, t)
	})

	t.Run("closu side-effect", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			f := func() {
				a = 2
				println(a)
			}
			println(a)
			f()
			println(a)
		}
		`, []string{
			"2","1","side-effect(2, a)",
		}, t)
	})

	t.Run("closu freevalue and global", func(t *testing.T) {

		test.CheckPrintlnValue(`package main
		
		var count = 1

		func newCounter() func() {
			return func() {
				println(count)
				count = 2
			}
		}

		func main(){
			f := newCounter()
			println(count)
			f()
			println(count)
		}

		`, []string{
			"1","1","1",
		}, t)
	})
}

func TestClosu_Value_InFunction(t *testing.T) {
	t.Run("closu function param", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

		func test (f func() int) int{
			return f()
		}

		func main(){
			a := 1
			test(func() int {
				println(a)
				return a
			})
			println(a)
		}
		`, []string{
			"FreeValue-a","1",
		}, t)
	})

	t.Run("closu function return", func(t *testing.T) {

		test.CheckPrintlnValue(`package main

			func test () func() int{
				var a = 1
				return func() int {
					println(a)
					return a
				}
			}

			func main(){
				f := test()
				a := f()
				println(a)
			}
		`, []string{
			"FreeValue-a","Function-test()() binding[1]",
		}, t)
	})
}

