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
		
		type s struct {
			a, b int
		}

		type i interface {
			Add() int
			Sub() int
		}

		func (i *s) Add() int {
			return i.a + i.b
		}

		func (i *s) Sub() int {
			return i.a - i.b
		}

		func do(i i) {
			println(i.Add())
			println(i.Sub())
		}

		func main(){
			b := &s{a: 3, b: 3}
			do(b)
		}
		`, []string{
			"6",
		}, t)
	})
}
