package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBuilder(t *testing.T) {
	t.Run("builder", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		func main(){
		 	println("hello world")
		}

		`, []string{
			`"hello world"`,
		}, t)
	})
}
