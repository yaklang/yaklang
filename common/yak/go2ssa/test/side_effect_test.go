package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Closu_SideEffect(t *testing.T) {
	t.Run("side-effect bind", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		
		func main(){
			a := 1
			f1 := func() {
				a = 2
			}
			f2 := func() {
				a := 3
				println(a) // 3
				f1()	   // f1产生的side-effect(2,a)与'a:=1'绑定,不会影响到'a:=3'
				println(a) // 3
			}
			println(a) // 1
			f2()
			println(a) // side-effect(2, a)
		}


		`, []string{"3", "3", "1", "side-effect(2, a)"}, t)
	})
}
