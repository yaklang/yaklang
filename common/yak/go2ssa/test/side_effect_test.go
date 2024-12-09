package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_SideEffect(t *testing.T) {
	t.Run("side-effect", func(t *testing.T) {
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

	t.Run("side-effect inherit", func(t *testing.T) {
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

	t.Run("side-effect muti value", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
	func main(){
		a := 1
		f1 := func() {
			a = 2
			a = 3
			a = 4
		}
		println(a) // 1
		f1()
		println(a) // side-effect(4, a)
	}
		`, []string{"1", "side-effect(4, a)"}, t)
	})

	t.Run("side-effect muti value with phi", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
	func main(){
		a := 1
		f1 := func() {
			a = 2
			a = 3
			a = 4
			if a == 1 {
			    a = 5
			}
		}
		println(a) // 1
		f1()
		println(a) // side-effect(phi(a)[5,4], a)
	}
		`, []string{"1", "side-effect(phi(a)[5,4], a)"}, t)
	})

	t.Run("side-effect method", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

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

	t.Run("side-effect nesting bind with local", func(t *testing.T) {
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
		test.CheckPrintlnValue(`package main

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
		test.CheckPrintlnValue(`package main

	func main(){
		a := 1
		f1 := func() {
			a = 2
		}
		{
			a := 3	 
			f2 := func() {
				f1()
				println(a) // freevalue
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

	t.Run("side-effect cross block nesting bind with local", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

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

	// Todo: side-effect in phi with different bind value
	t.Run("side-effect cross block nesting bind with phi", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`package main

	func main(){
		a := 1
		f1 := func() {
			a = 2
		}
		{
			a := 3	 
			f2 := func() {
				if true{
					f1()
				}else{
					a = 4
				}
				println(a) // phi(a)[FreeValue-a,4]
			}
			println(a) // 3
			f2()
			println(a) // side-effect(phi(a)[3,4], a)
		}
		println(a) // side-effect(phi(a)[1,4], a)
	}
		`, []string{
			"phi(a)[FreeValue-a,4]", "3", "side-effect(phi(a)[3,4], a)", "side-effect(phi(a)[1,4], a)",
		}, t)
	})

	t.Run("side-effect cross block nesting bind with local and phi", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

	func main(){
		a := 1
		f1 := func() {
			a = 2
		}
		{
			a := 3	 
			f2 := func() {
				a := 4
				if a == 4 {
				    a = 5
				}
				f1()
				println(a) // phi(a)[5,4]
			}
			println(a) // 3
			f2()
			println(a) // 3
		}
		println(a) // side-effect(2, a)
	}
		`, []string{
			"phi(a)[5,4]", "3", "3", "side-effect(2, a)",
		}, t)
	})

	t.Run("side-effect cross global", func(t *testing.T) {
		// TODO: handle global and side-effect
		t.Skip()
		test.CheckPrintlnValue(`package main

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

func Test_SideEffect_Return(t *testing.T) {
	t.Run("side-effect with full path", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
		    a := 0
			f := func() {
			    if true {
			        a = 2
			    }else{
					a = 3
				}
				println(a)
			}
			a = 1
			f()
			println(a)
		}
		`, []string{
			"phi(a)[2,3]", "side-effect(phi(a)[2,3], a)",
		}, t)
	})

	t.Run("side-effect with empty path", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
		    a := 0
			f := func() {
			    if true {
			        a = 2
			    }else{
	
				}
				println(a)
			}
			a = 1
			f()
			println(a)
		}
		`, []string{
			"phi(a)[2,FreeValue-a]", "side-effect(phi(a)[2,1], a)",
		}, t)
	})
	t.Run("side-effect with empty path extend", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
		    a := 0
			f := func() {
			    if true {
			        a = 2
			    }else{
	
				}
				println(a)
			}
			a = 1
			f()
			println(a)
			a = 3
			f()
			println(a)
		}
		`, []string{
			"phi(a)[2,FreeValue-a]", "side-effect(phi(a)[2,1], a)", "side-effect(phi(a)[2,3], a)",
		}, t)
	})

	t.Run("side-effect bind with closu", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			f := func() {
			 	a = 1
				f1 := func() {
					a = 2
				}

				if c {
					a = 3
				} else {
					f1() 
				}
				println(a) // phi(a)[3,side-effect(2, a)]
			}
		}
		`, []string{
			"phi(a)[3,side-effect(2, a)]",
		}, t)
	})

	t.Run("side-effect nesting bind with closu", func(t *testing.T) {
		// Todo: 从CapturedSideEffect获取的side-effect忽略了cfg信息，导致没有生成phi值
		t.Skip()
		test.CheckPrintlnValue(`package main

		func main(){
			f := func() {
				a = 1 
				f1 := func() {
					a = 2
				}
				{
					a = 3 
					if c {
						a = 4
					} else {
						f1() 
					}
					println(a) // phi(a)[4,3]
				}

				println(a) // phi(a)[1,2]
			}
		}
		`, []string{
			"phi(a)[4,3]", "phi(a)[1,2]",
		}, t)
	})
}
