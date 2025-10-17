package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBlock_Normol(t *testing.T) {
	t.Run("cross block", func(t *testing.T) {
		ssatest.CheckPrintlnValue(`package main

	import "fmt"

	func test() {
		a := 1
		{
			a := 2
			a = 4
			println(a)
		}
		println(a) 
	}
		`, []string{
			"4", "1",
		}, t)
	})
}

func TestBlock_Value_If(t *testing.T) {
	t.Run("if stmt;exp", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
		 	if a := 2; a > 1 {
				println(a) // 2
				a = 3
		 	}else{
				println(a) // 2
				a = 4
		 	}
		 	println(a) // Undefined-a
		}
		`, []string{
			"2", "2", "Undefined-a",
		}, t)
	})

	t.Run("if stmt;exp EX", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			a := 1
		 	if a := 2; a > 1 {
				println(a) // 2
		 	}else{
				println(a) // 2
		 	}
		 	println(a) // 1
		}
		`, []string{
			"2", "2", "1",
		}, t)
	})

	t.Run("if stmt;exp EX2", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			a := 1
		 	if a = 2; a > 1 {
				println(a) // 2
		 	}else{
				println(a) // 2
		 	}
		 	println(a) // 2
		}
		`, []string{
			"2", "2", "2",
		}, t)
	})

	t.Run("if stmt;exp and block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			b := 2
			{
				b = 3
				if a = 2; a > 1 {
				    println(a) // 2
				}
			}
			println(b) // 3
		}
		`, []string{
			"2", "3",
		}, t)
	})

	t.Run("if-else stmt;exp", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			if i:=1; i==1 {
				println(i) // 1
			}else if a:=2; a==2{ 
				println(i) // 1
			}else{
				println(a) // 2
				println(i) // 1
			}
		}
		`, []string{
			"1", "1", "2", "1",
		}, t)
	})

	t.Run("if-else stmt;exp EX", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			if i:=1; i==1 {
				println(i)
			}else if i:=2; i==2{ 
				println(i)
			}else{
				println(i)
			}
		}
		`, []string{
			"1", "2", "2",
		}, t)
	})

	t.Run("if-else stmt;exp EX2", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			i := 1
			if i := 2; i==1 {
				println(i)	// 2
			}else if i = 3; i==2{ 
				println(i)	// 3
			}else{
				println(i)	// 3
			}
			println(i)	// 1
		}
		`, []string{
			"2", "3", "3", "1",
		}, t)
	})

	t.Run("if-else stmt;exp EX3", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			i := 1
			if i = 2; i==1 {
				println(i)	// 2
			}else if i = 3; i==2{ 
				println(i)	// 3
			}else{
				println(i)	// 3
			}
			println(i)	// 3
		}
		`, []string{
			"2", "3", "3", "3",
		}, t)
	})

	t.Run("if-else stmt;exp and block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			b := 2
			{
				b = 3
				if a = 2; a > 1 {
				    println(a) // 2
				}else if a = 3; a > 2{
				    println(a) // 3
				}
			}
			println(b) // 3
		}
		`, []string{
			"2", "3", "3",
		}, t)
	})
}

func TestBlock_Value_Switch(t *testing.T) {
	t.Run("switch stmt;exp", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
		func main(){
			switch a := 2; a {
			default:
				println(a) // 2
			}
			println(a) // Undefined-a
		}
		`, []string{"2", "Undefined-a"}, t)
	})

	t.Run("switch stmt;exp EX", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
		func main(){
			a := 1
			switch a = 2; a {
			case 2:
				a = 1
			default:
				println(a) // 2
			}
			println(a) // 2
		}
		`, []string{"2", "phi(a)[1,2]"}, t)
	})

	t.Run("switch stmt;exp EX2", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
		func main(){
			a := 1
			switch a := 2; a {
			case 2:
				a = 1
			default:
				println(a) // 2
			}
			println(a) // 1
		}
		`, []string{"2", "1"}, t)
	})

	t.Run("switch stmt;exp EX3", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			switch a = 2; a {
			default:
				println(a) // 2
			}
			println(a) // 2
		}
		`, []string{"2", "2"}, t)
	})

	t.Run("switch stmt;exp and block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
		func main(){
			a := 1
			b := 2
			{
				b = 3
				switch a = 2; a {
				default:
					println(a) // 2
				}
			}
			println(b) // 3
		}
		`, []string{"2", "3"}, t)
	})
}

func TestBlock_Value_Select(t *testing.T) {
	t.Run("select recv", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			channel1 := make(chan int)
			channel2 := make(chan int)

		    select {
			case data1 := <-channel1:
				println(data1)
			case data2 := <-channel2:
				println(data2)
			default:
			}
		}
		`, []string{"chan(make(chan number))", "chan(make(chan number))"}, t)
	})

	// TODO: select send
	/*
		t.Run("select send", func(t *testing.T) {
			test.CheckPrintlnValue( `package main

			func main(){
				channel1 := make(chan int)
				channel2 := make(chan int)

			    select {
				case channel1 <- 1:
				case channel2 <- 2:
				default:
				}
			}
			`, []string{""}, t)
		})*/
}

func TestBlock_Value_For(t *testing.T) {
	t.Run("for stmt;exp;", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			i := 0
			for i = 1; i < 10; {
				println(i) // 1
			}
			println(i) // 1
		}
		`, []string{"1", "1"}, t)
	})

	t.Run("for stmt;exp;stmt", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			for i := 1; i < 10; i++ {
				println(i) // phi
			}
			println(i) // Undefined-i
		}
		`, []string{"phi(i)[1,add(i, 1)]", "Undefined-i"}, t)
	})

	t.Run("for stmt;exp;stmt EX", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			i := 10
			for i := 5; i < 10; i++ {
				println(i) // phi
			}
			println(i) // 10
		}
		`, []string{"phi(i)[5,add(i, 1)]", "10"}, t)
	})

	t.Run("for stmt;exp;stmt EX2", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			i := 10
			for i := 5; i < 10; i++ {
				println(i) // phi
				i = 10
			}
			println(i) // 10
		}
		`, []string{"phi(i)[5,11]", "10"}, t)
	})

	t.Run("for stmt;exp;stmt EX3", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			a := 1
			i := 10
			for i := 5; i < 10; i++ {
				a = 10
			}
			println(i) // 10
			println(a) // phi
		}
		`, []string{"10", "phi(a)[1,10]"}, t)
	})

	t.Run("for stmt;exp;stmt EX4", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			a := 1
			i := 10
			for i = 5; i < 10; i++ {
				a := 10
				println(i) // phi
			}
			println(i) // phi
			println(a) // 1
		}
		`, []string{"phi(i)[5,add(i, 1)]", "phi(i)[5,add(i, 1)]", "1"}, t)
	})

	t.Run("for stmt;exp;stmt and block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			b := 2
			{
				b = 3
				for a = 1; a < 10; a++ {
					println(a) // phi
				}
			}
			println(b) // 3
		}
		`, []string{"phi(a)[1,add(a, 1)]", "3"}, t)
	})
}

func TestBlock_Return_Phi(t *testing.T) {
	t.Run("phi-with-return", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main() {
			a := 1
			if true {
				return
			}
			println(a) // phi
		}
		`, []string{"phi(a)[Undefined-a,1]"}, t)
	})

	t.Run("phi-with-return-nested-if", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main() {
			a := 1
			if true {
				if false {
				    return
				}
			}
			println(a) // phi
		}
		`, []string{"phi(a)[phi(a)[Undefined-a,1],1]"}, t)
	})

	t.Run("phi-with-return-if-else", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main() {
			a := 1
			if true {
				return
			}else if false {
				return
			}
			println(a) // phi
		}
		`, []string{"phi(a)[Undefined-a,Undefined-a,1]"}, t)
	})

	t.Run("phi-with-return-nested-if-else", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main() {
			a := 1
			if true {
				if false {
				    return
				}else if false {
				    return
				}
			}
			println(a) // phi
		}
		`, []string{"phi(a)[phi(a)[Undefined-a,Undefined-a,1],1]"}, t)
	})
}
