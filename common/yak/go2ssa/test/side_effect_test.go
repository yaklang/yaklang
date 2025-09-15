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

	t.Run("value with phi", func(t *testing.T) {
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
			"side-effect(2, t.a)",
		}, t)
	})

	t.Run("side-effect bind normal", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
	func main(){
		n := 1
		b := func() {
			n = 2 // modify
		}
		{
			n = 3
			b()
			println(n)
		}
		println(n)
	}
		`, []string{"side-effect(2, n)", "side-effect(2, n)"}, t)

		test.CheckPrintlnValue(`package main
		
		func main(){
			n := 1
			b := func() {
				n = 2 // modify
			}
			{
				n := 3
				b()
				println(n)
			}
			println(n)
		}
			`, []string{"3", "side-effect(2, n)"}, t)
	})

	t.Run("side-effect bind with local", func(t *testing.T) {
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

	t.Run("side-effect cross block nesting bind with phi", func(t *testing.T) {
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
			println(a) // side-effect(phi(a)[FreeValue-a,4], a)
		}
		println(a) // side-effect(phi(a)[side-effect(2, a),FreeValue-a], a)
	}
		`, []string{
			"phi(a)[FreeValue-a,4]", "3", "side-effect(phi(a)[FreeValue-a,4], a)", "side-effect(phi(a)[side-effect(2, a),FreeValue-a], a)",
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

func Test_SideEffect_Object(t *testing.T) {
	t.Run("side-effect object normol", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		type T struct {
			a int
			b int
		}
		func main(){
			o := &T{a: 1, b: 2}
			f1 := func() {
				o.a = 2
			}
			f1()
			println(o.a)
		}
		`, []string{"side-effect(2, o.a)"}, t)
	})

	t.Run("side-effect object copy", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		type T struct {
			a int
			b int
		}
		func main(){
			o1 := T{a: 1, b: 2}
			o2 := o1
			f1 := func() {
				o1.a = 2
			}
			f1()
			println(o1.a)
			println(o2.a)
		}
		`, []string{"side-effect(2, o1.a)", "1"}, t)
	})

	// 等待pr：https://github.com/yaklang/yaklang/pull/2395
	t.Run("side-effect pointer copy", func(t *testing.T) {
		t.Skip()
		test.CheckPrintlnValue(`package main
		type T struct {
			a int
			b int
		}
		func main(){
			o1 := T{a: 1, b: 2}
			o2 := &o1
			f1 := func() {
				o1.a = 2
			}
			f1()
			println(o1.a)
			println(o2.a)
		}
		`, []string{"side-effect(2, o1.a)", "side-effect(2, o1.a)"}, t)
	})

	t.Run("side-effect object without init", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		type T struct {
			a int
			b int
		}
		func main(){
			o := &T{}
			f1 := func() {
				o.a = 2
			}
			f1()
			println(o.a)
		}
		`, []string{"side-effect(2, o.a)"}, t)
	})

	// todo: side-effect object undefined
	t.Run("side-effect object assign", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		type T struct {
			a int
			b int
		}
		func main(){
			o := &T{a: 1, b: 2}
			f1 := func() {
				o = &T{a: 3, b: 4}
			}
			f1()
			println(o.a)
		}
		`, []string{"Undefined-o.a(valid)"}, t)
	})

	t.Run("side-effect object assign in if", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		type T struct {
			a int
			b int
		}
		func main(){
			o := &T{a: 1, b: 2}
			f1 := func() {
				if true {
					o = &T{a: 3, b: 4}
				}
			}
			f1()
			println(o.a)
		}
		`, []string{"Undefined-o.a(valid)"}, t)
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
				println(a) // phi(a)[2,3]
			}
			a = 1
			f()
			println(a) // side-effect(phi(a)[2,3], a)
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
				println(a) // phi(a)[2,FreeValue-a]
			}
			a = 1
			f()
			println(a) // side-effect(phi(a)[2,FreeValue-a], a)
		}
		`, []string{
			"phi(a)[2,FreeValue-a]", "side-effect(phi(a)[2,FreeValue-a], a)",
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
				println(a) // phi(a)[2,FreeValue-a]
			}
			a = 1
			f()
			println(a) // side-effect(phi(a)[2,FreeValue-a], a)
			a = 3
			f()
			println(a) // side-effect(phi(a)[2,FreeValue-a], a)
		}
		`, []string{
			"phi(a)[2,FreeValue-a]", "side-effect(phi(a)[2,FreeValue-a], a)", "side-effect(phi(a)[2,FreeValue-a], a)",
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
					println(a) // phi(a)[4,side-effect(2, a)]
				}

				println(a) // phi(a)[4,side-effect(2, a)]
			}
		}
		`, []string{
			"phi(a)[4,side-effect(2, a)]", "phi(a)[4,side-effect(2, a)]",
		}, t)
	})

	t.Run("side-effect nesting bind with closu and local", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			f := func() {
				a = 1 
				f1 := func() {
					a = 2
				}
				{
					a := 3 
					if c {
						a = 4
					} else {
						f1() 
					}
					println(a) // phi(a)[4,3]
				}

				println(a) // phi(a)[1,side-effect(2, a)]
			}
		}
		`, []string{
			"phi(a)[4,3]", "phi(a)[1,side-effect(2, a)]",
		}, t)
	})

	t.Run("side-effect nesting bind with closu and local cross block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			f := func() {
				a = 1 
				f1 := func() {
					a = 2
				}
				{
					a := 3 
					{
						a = 4 
						if c {
							a = 5
						} else {
							f1() 
						}
						println(a) // phi(a)[5,4]
					}
				}

				println(a) // phi(a)[1,side-effect(2, a)]
			}
		}
		`, []string{
			"phi(a)[5,4]", "phi(a)[1,side-effect(2, a)]",
		}, t)
	})

	t.Run("side-effect nesting bind with closu and local cross empty block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			f := func() {
				a = 1 
				f1 := func() {
					a = 2
				}
				{
					a := 4 
					{
						if c {
							a = 5
						} else {
							f1() 
						}
						println(a) // phi(a)[5,4]
					}
				}

				println(a) // phi(a)[1,side-effect(2, a)]
			}
		}
		`, []string{
			"phi(a)[5,4]", "phi(a)[1,side-effect(2, a)]",
		}, t)
	})
}

func Test_SideEffect_MutiReturn(t *testing.T) {
	t.Run("muti return", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			f := func(){
				if c {
					a = 2
					return
				}
				a = 3
				println(a) // 3
			}
			f()
			println(a) // side-effect(phi(a)[2,3], a)
		}
		`, []string{"3", "side-effect(phi(a)[2,3], a)"}, t)
	})

	t.Run("different variable", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
		func main(){
			a := 1
			b := 1
			f := func(){
				if c {
					a = 2
					return
				}
				a = 3
				b = 4
				println(a) // 3
			}
			f()
			println(a) // side-effect(phi(a)[2,3], a)
			println(b) // side-effect(4, b)
		}
			`, []string{"3", "side-effect(phi(a)[2,3], a)", "side-effect(4, b)"}, t)
	})

	t.Run("different variable(same name)", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			f1 := func(){
				a = 2
			}
			{
				a = 3
				f2 := func(){
					if c {
						f1()
						return
					}
					println(a) // phi(a)[Undefined-a,FreeValue-a]
					a = 4
					println(a) // 4
				}
				f2() 
				println(a) // side-effect(phi(a)[side-effect(2, a),4], a)
			}
			println(a) // side-effect(phi(a)[side-effect(2, a),4], a)
		}
		`, []string{"phi(a)[Undefined-a,FreeValue-a]", "4", "side-effect(phi(a)[side-effect(2, a),4], a)", "side-effect(phi(a)[side-effect(2, a),4], a)"}, t)
	})

	t.Run("different variable(same name) have local", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
		func main(){
			a := 1
			f1 := func(){
				a = 2
			}
			{
				a := 3
				f2 := func(){
					if c {
						f1()
						return
					}
					println(a) // phi(a)[Undefined-a,FreeValue-a]
					a = 4
					println(a) // 4
				}
				f2() 
				println(a) // side-effect(4, a)
			}
			println(a) // side-effect(2, a)
		}
		`, []string{"phi(a)[Undefined-a,FreeValue-a]", "4", "side-effect(4, a)", "side-effect(2, a)"}, t)
	})

	t.Run("last return", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
	
		func main(){
			a := 1
			f1 := func(){
				a = 2
				return
			}
			f1()
			println(a) // side-effect(2, a)
		}
		`, []string{"side-effect(2, a)"}, t)
	})
}
