package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBasic_Variable_Inblock(t *testing.T) {
	t.Run("test simple assign", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main() {
			a := 1
			println(a)
			a := 2
			println(a)
		}
	`, []string{
			"1",
			"2",
		}, t)
	})

	t.Run("test sub-scope capture parent scope in basic block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			println(a) // 1
			{
				a = 2
				println(a) // 2
			}
			println(a) // 2
		}
		`, []string{
			"1",
			"2",
			"2",
		}, t)
	})

	t.Run("test sub-scope var local variable in basic block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			println(a) // 1
			{
				var a int = 2
				println(a) // 2
			}
			println(a) // 1
		}
		`, []string{
			"1",
			"2",
			"1",
		}, t)
	})

	t.Run("test sub-scope var local variable without assign in basic block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			println(a) // 1
			{
				var a int
				println(a) // any
			}
			println(a) // 1
		}
		`, []string{
			"1",
			"0",
			"1",
		}, t)
	})

	t.Run("test sub-scope local variable in basic block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a = 1
			println(a) // 1
			{
				a := 2
				println(a) // 2
			}
			println(a) // 1
		}
		`, []string{
			"1",
			"2",
			"1",
		}, t)
	})

	t.Run("test sub-scope and return", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			println(a) // 1
			{
				a = 2 
				println(a) // 2
				return 
			}
			println(a) // unreachable
		}

		`,
			[]string{
				"1", "2",
			}, t)
	})

	t.Run("variable in sub-scope", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			{
				a := 2
				println(a) // 2
			}
			println(a) // Undefined-a
		}
		`, []string{
			"2",
			"Undefined-a",
		}, t)
	})

	t.Run("test ++ expression", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
    		a := 1
			{
				a++
				println(a) // 2
			}
		}
		`,
			[]string{
				"2",
			},
			t)
	})

	t.Run("test syntax block lose capture variable", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
    		a := 1 
			{
				a = 2  // capture [a: 2]
				{
					println(a) // 2
				} 
				// end-scope capture is []
			}
			println(a) // 2
		}
		`, []string{
			"2", "2",
		}, t)
	})

	t.Run("test assign cross block", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main() {
			a := 1
			{
				a = 2
				println(a)
				a := 3
				println(a)
				a = 4
				println(a)
			}
			println(a)
		}
	`, []string{
			"2", "3", "4", "2",
		}, t)
	})

}

func TestBasic_Variable_InIf(t *testing.T) {
	t.Run("test simple if", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			var a int
			a = 1
			println(a)
			if c {
				a = 2
				println(a)
			}
			println(a)
		}
		`, []string{
			"1",
			"2",
			"phi(a)[2,1]",
		}, t)
	})
	t.Run("test simple if with local vairable", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
    		a := 1
			println(a) // 1
			if c {
				a := 2
				println(a) // 2
			}
			println(a) // 1
		}
		`, []string{
			"1",
			"2",
			"1",
		}, t)
	})

	t.Run("test multiple phi if", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			if c {
				a = 2
			}
			println(a)
			println(a)
			println(a)
		}
		`, []string{
			"phi(a)[2,1]",
			"phi(a)[2,1]",
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("test multiple if ", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			if 1 {
				if 2 {
					a = 2
				}
			}
			println(a)
		}

	`,
			[]string{
				"phi(a)[phi(a)[2,1],1]",
			},
			t)
	})

	t.Run("test simple if else", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
    		a := 1
			println(a)
			if c {
				a = 2
				println(a)
			} else {
				a = 3
				println(a)
			}
			println(a)
		}
		`, []string{
			"1",
			"2",
			"3",
			"phi(a)[2,3]",
		}, t)
	})

	t.Run("test simple if else with origin branch", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
    		a := 1
			println(a)
			if c {
				// a = 1
			} else {
				a = 3
				println(a)
			}
			println(a) // phi(a)[1, 3]
		}
		`, []string{
			"1",
			"3",
			"phi(a)[1,3]",
		}, t)
	})

	t.Run("test if-elseif", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			println(a)
			if c {
				a = 2
				println(a)
			}else if  c == 2{
				a = 3
				println(a)
			}
			println(a)
		}
		`,
			[]string{
				"1",
				"2",
				"3",
				"phi(a)[2,3,1]",
			}, t)
	})
}

func TestBasic_Variable_If_Return(t *testing.T) {
	t.Run("test with return, no DoneBlock", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
    		a := 1
			println(a) // 1
			if c {
				a = 2
				return 
			}
			println(a) // phi(a)[Undefined-a,1]
		}
		`, []string{
			"1",
			"phi(a)[Undefined-a,1]",
		}, t)
	})
	t.Run("test with return in branch, no DoneBlock", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			println(a) // 1
			if c {
				if b {
					a = 2
					println(a) // 2
					return 
				}else {
					a = 3
					println(a) // 3
					return 
				}
				println(a) // unreachable // phi[2, 3]
			}
			println(a) // phi(a)[Undefined-a,1]
		}

		`, []string{
			"1",
			"2",
			"3",
			"phi(a)[Undefined-a,1]",
		}, t)
	})
}

func TestBasic_Variable_If_Logical(t *testing.T) {
	t.Run("test simple", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			if c || b {
				a = 2
			}
			println(a)
		}
		`, []string{"phi(a)[2,1]"}, t)
	})

	t.Run("test multiple logical", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			if c || b && d {
				a = 2
			}
			println(a)
		}
		`, []string{"phi(a)[2,1]"}, t)
	})
}

func TestBasic_Variable_Loop(t *testing.T) {
	t.Run("simple loop not change", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			for i:=0; i < 10 ; i++ {
				println(a) // 1
			}
			println(a) //1 	
		}
		`,
			[]string{
				"1",
				"1",
			},
			t)
	})

	t.Run("simple loop only condition", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			i := 1
			for i < 10 { 
				println(i) // phi
				i = 2 
				println(i) // 2
			}
			println(i) // phi
		}
		`, []string{
			"phi(i)[1,2]",
			"2",
			"phi(i)[1,2]",
		}, t)
	})

	t.Run("simple loop", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			i:=0
			for i=0; i<10; i++ {
				println(i) // phi[0, i+1]
			}
			println(i)
		}
		`,
			[]string{
				"phi(i)[0,add(i, 1)]",
				"phi(i)[0,add(i, 1)]",
			}, t)
	})

	t.Run("loop with spin, signal phi", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			for i := 0; i < 10; i ++ { // i=0; i=phi[0,1]; i=0+1=1
				println(a) // phi[0, $+1]
				a = 0
				println(a) // 0 
			}
			println(a)  // phi[0, 1]
		}
		`,
			[]string{
				"phi(a)[1,0]",
				"0",
				"phi(a)[1,0]",
			},
			t)
	})

	t.Run("loop with spin, double phi", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			for i := 0; i < 10; i ++ {
				a += 1
				println(a) // add(phi, 1)
			}
			println(a)  // phi[1, add(phi, 1)]
		}
		`,
			[]string{
				"add(phi(a)[1,add(a, 1)], 1)",
				"phi(a)[1,add(a, 1)]",
			},
			t)
	})
}

func TestBasic_Variable_Switch(t *testing.T) {
	t.Run("simple switch, no default", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
		    a := 1
			switch a {
			case 2: 
				a = 22
				println(a)
			case 3, 4:
				a = 33
				println(a)
			}
			println(a) // phi[1, 22, 33]
		}
		`, []string{
			"22", "33", "phi(a)[22,33,1]",
		}, t)
	})

	t.Run("simple switch, has default but nothing", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
		    a := 1
			switch a {
			case 2: 
				a = 22
				println(a)
			case 3, 4:
				a = 33
				println(a)
			default: 
			}
			println(a) // phi[1, 22, 33]
		}
		`, []string{
			"22", "33", "phi(a)[22,33,1]",
		}, t)
	})

	t.Run("simple switch, has default", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
		    a := 1
			switch a {
			case 2: 
				a = 22
				println(a)
			case 3, 4:
				a = 33
				println(a)
			default: 
				a = 44
				println(a)
			}
			println(a) // phi[22, 33, 44]
		}
		`, []string{
			"22", "33", "44", "phi(a)[22,33,44]",
		}, t)
	})
}

func TestBasic_Variable_Select(t *testing.T) {
	t.Run("simple select", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			channel1 := make(chan int)
			channel2 := make(chan string)

		    select {
			case data1 := <-channel1:
				println(data1) // chan(Function-make(typeValue(chan number)))
			case data2 := <-channel2:
				println(data1) // Undefined-data1
			}
		}
		`, []string{
			"chan(make(chan number))", "Undefined-data1",
		}, t)
	})

	t.Run("simple select phi-case", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main() {
			data1 := "hello"

			channel1 := make(chan string)
			channel2 := make(chan string)

			select {
			case data1 = <-channel1:
			case data1 = <-channel2:
			}

			println(data1)
		}
		`, []string{
			"phi(data1)[chan(make(chan string)),chan(make(chan string)),\"hello\"]",
		}, t)
	})

	t.Run("simple select cover-case", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main() {
			data1 := "hello"
			channel1 := make(chan string)

			select {
			case data1 = <-channel1: // cover
				data1 = "world"
			}
			println(data1)
		}
		`, []string{
			"phi(data1)[\"world\",\"hello\"]",
		}, t)
	})

	t.Run("simple select phi-cover-case", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main() {
			data1 := "111111"
			channel1 := make(chan string)

			select {
			case data1 = <-channel1: // cover
				data1 = "333333" 
			case data1 = <-channel1: // phi
								
			}

			println(data1)
		}
		`, []string{
			"phi(data1)[\"333333\",chan(make(chan string)),\"111111\"]",
		}, t)
	})

	t.Run("if select phi", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main() {
			data1 := 1
			channel1 := make(chan int)

			select {
			case data1 = <-channel1: // cover
				data1 = 2 
			case data1 = <-channel1: // phi
				if data1 == 0 { // phi
					data1 = 3
				}
			}

			println(data1)
		}
		`, []string{
			"phi(data1)[2,phi(data1)[3,chan(make(chan number))],1]",
		}, t)
	})
}

func TestBasic_CFG_Break(t *testing.T) {
	t.Run("simple break in loop", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			a := 1
			for i := 0; i < 10; i++ {
				if i == 5 {
					a = 2
					break
				}
			}
			println(a) // phi[1, 2]
		}
		`, []string{
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("simple continue in loop", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
			a := 1
			for i := 0; i < 10; i++ {
				if i == 5 {
					a = 2
					continue
				}
			}
			println(a) // phi[1, 2]
		}
		`, []string{
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("simple break in switch", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a := 1
			switch a {
			case 1:
				if c {
					a = 2
					break
				}
				a = 4
			case 2:
				a = 3
		}
		println(a) // phi[1, 2, 3, 4]
		}
		`, []string{
			"phi(a)[2,4,3,1]",
		}, t)
	})

	t.Run("simple fallthrough in switch", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
			a = 1
			switch a {
			case 1:
				a = 2
				fallthrough
			case 2:
				println(a) // 1 2
				a = 3
			default: 
				a = 4
			}
			println(a) // 3 4
		}
		`, []string{
			"phi(a)[2,1]",
			"phi(a)[3,4]",
		}, t)
	})
}

func TestBasic_CFG_Goto(t *testing.T) {
	/*t.Run("simple goto up", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main() {
			a := 1
			if a > 1 {
				println(a) // 1
		end:
				println(a) // phi(a)[1,5]
			}else{
				a = 5
				goto end
			}
		}
		`, []string{
			"1", "phi(a)[1,5]",
		}, t)
	})*/

	t.Run("goto down in if", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main() {
			a := 1
			if a > 1 {
				a = 5
				goto end
			}else{
				println(a) // 1
		end:
				println(a) // phi(a)[1,5]
			}
		}
		`, []string{
			"1", "phi(a)[1,5]",
		}, t)
	})

	t.Run("goto down in loop", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main() {
			a := 1

			for i:=0; i<10; i++ {
				a = 2
				goto label1
			}
			println(a) // phi(a)[1,2]
			label1:
			println(a) // phi(a)[phi(a)[1,2],2]
		}
		`, []string{
			"phi(a)[1,2]", "phi(a)[phi(a)[1,2],2]",
		}, t)
	})

	t.Run("break label in loop", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main() {
			a := 1

			label1:
			for i:=0; i<10; i++ {
				a = 2
				break label1
			}
			println(a) // phi(a)[phi(a)[1,2],2]
		}
		`, []string{
			"phi(a)[phi(a)[1,2],2]",
		}, t)
	})

	t.Run("muti break label in loop", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main() {
			a := 1

			label1:
			for i:=0; i<10; i++ {
				for y:=0; y<10; y++ {
					a = 2
					break label1
				}
			}
			println(a) // phi(a)[phi(a)[1,2],2]
		}
		`, []string{
			"phi(a)[phi(a)[1,2],2]",
		}, t)
	})
}

func TestBasic_CFG_Defer(t *testing.T) {
	t.Run("simple defer", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func main(){
		    a := 1
			defer func(){
				a = 2
				println(a)
			}()

			println(a)
		}
		`, []string{
			"2", "1",
		}, t)
	})
}
