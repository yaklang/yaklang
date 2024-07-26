package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBuilder(t *testing.T) {
	t.Run("builder", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
		 	println("hello world")
		}

		`, []string{
			`"hello world"`,
		}, t)
	})
}

func TestTemp(t *testing.T) {
	t.Run("temp", func(t *testing.T) {
		test.CheckPrintlnValue( `package main

		type mystruct struct{
		    a int 
			b string
		}

		func main(){
			t := mystruct{a:1,b:"hello",c:[]int{1,2,3}}
			println(t.a)
			println(t.b)
			println(t.c[2])
		}
		`, []string{
			``,
		}, t)
	})
}
