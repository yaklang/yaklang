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

		func main(){
			var x = 10
			x += 100
			
			switch x.(type) {
			case type1:
			case type2:
			default:
			}
		}

		`, []string{
			``,
		}, t)
	})
}


