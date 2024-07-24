package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestClosure_FreeValue_Value(t *testing.T) {

	t.Run("normal function", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func a(){
			a := 1
			println(a)
		}
		func main(){
			a()
		}
		`, []string{
			"1",
		}, t)
	})

	t.Run("normal function freeValue", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func a(){
			println(a)
		}
		func main(){
			a()
		}
		`, []string{
			"FreeValue-a",
		}, t)
	})

	t.Run("golbal value", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		var a = 1
		func a(){
			println(a)
		}
		func main(){
			a()
		}
		`, []string{
			"FreeValue-a",
		}, t)
	})

	t.Run("golbal loval value", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
		var a = 1
		func a(){
			var a = 2
			println(a)
		}
		func main(){
			a()
			println(a)
		}
		`, []string{
			"2","FreeValue-a",
		}, t)
	})
}