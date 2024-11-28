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
			"Function-add", "Function-add(1,2)",
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
}

func TestFunction_GlobalValue(t *testing.T) {
	t.Run("global value", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		var count = 1

		func main(){
			println(count) // 1
		}
		`, []string{
			"1",
		}, t)
	})

	t.Run("global value phi", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
		var count = 1
	
		func main(){
			if true {
			    count = 2
				println(count) // 2
			}

			println(count) // phi(count)[2,1]
		}
		`, []string{
			"2", "phi(count)[2,1]",
		}, t)
	})

	t.Run("global value phi scope", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
		var count = 1
	
		func main(){
			if true {
			    count = 2
				count = 3
				println(count) // 3
			}

			println(count) // phi(count)[3,1]
		}
		`, []string{
			"3", "phi(count)[3,1]",
		}, t)
	})

	t.Run("global value phi scope sub", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
		var count = 1
	
		func main(){
			if true {
			    count = 2
				{
					count = 3
				}
				println(count) // 3
			}

			println(count) // phi(count)[3,1]
		}
		`, []string{
			"3", "phi(count)[3,1]",
		}, t)
	})

	t.Run("global value phi merge", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		var count = 1

		func main(){
			count = 2
			if true {
			    count = 3
			}else{
			    count = 4
			}
			println(count) // phi(count)[3,4]
		}

		func main2(){
		    println(count) // 1
		}
		`, []string{
			"phi(count)[3,4]", "1",
		}, t)
	})

	t.Run("global value phi mergeEX", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		var count = 1

		func main(){
			if true {
			    count = 3
			}else{
			    count = 4
			}
			count = 5
			println(count) // 5
		}

		func main2(){
		    println(count) // 1
		}
		`, []string{
			"5", "1",
		}, t)
	})

	t.Run("global value phi function", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
		var count = 1
	
		func f(){
			count = 2
			println(count) // 2
		}
	
		func main(){
			println(count) // 1
			if true {
			    count = 3
			}else{
			    count = 4
			}
			println(count) // phi(count)[3,4]
		}
		`, []string{
			"2", "1", "phi(count)[3,4]",
		}, t)
	})

	t.Run("global value phi function-if", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
		var count = 1
	
		func f1(){
			count = 2
			println(count) // 2
		}

		func f2(){
			if true {
			    count = 3
			}
			println(count) // phi(count)[3,1]
		}

		func main(){
			println(count) // 1
		}	

		`, []string{
			"1", "2", "phi(count)[3,1]",
		}, t)
	})

	t.Run("global value phi function-loop", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
		var count = 1
	
		func f1(){
			count = 2
			println(count) // 2
		}

		func f2(){
			for count = 3; count > 0; count-- {
			}
			println(count) // phi(count)[3,sub(count, 1)]
		}

		func main(){
			println(count) // 1
		}	

		`, []string{
			"2", "phi(count)[3,sub(count, 1)]", "1",
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
			"FreeValue-a", "1",
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
			"2", "1", "side-effect(2, a)",
		}, t)
	})

	t.Run("closu freevalue and global", func(t *testing.T) {
		// TODO: global value
		t.Skip()
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
			"1", "phi(count)[phi(count)[1,2],1]", "phi(count)[phi(count)[1,2],1]",
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
			"FreeValue-a", "1",
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
			"FreeValue-a", "Function-test()() binding[1]",
		}, t)
	})
}
