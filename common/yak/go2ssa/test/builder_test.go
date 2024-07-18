package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBuilder(t *testing.T) {
	t.Run("normal function", func(t *testing.T) {
		test.CheckPrintlnValue( `package main
		func main(){
			var a = "hello world"
			println(a)
		}

		`, []string{
			"\"hello world\"",
		}, t)
	})
}