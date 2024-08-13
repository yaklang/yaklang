package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Express(t *testing.T) {
	t.Run("add", func(t *testing.T) {
		code := `package main

			func main(){
				a := 1 + 2
			}
		`
		ssatest.CheckSyntaxFlow(t, code, `
		a #-> as $target
		`, map[string][]string{
			"target": {"1", "2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}

/* // TODO: select send and recv
func Test_Statement(t *testing.T) {
	t.Run("select send and recv", func(t *testing.T) {
		code := `package main

		func println(){}

		func main(){
			channel1 := make(chan int)
			channel2 := make(chan int)

			go func() {
				channel1 <- 1 
				channel2 <- 2
			}()

		    select {
			case data1 := <-channel1:
				println(data1)
			case data2 := <-channel2:
				println(data2)
			default:
			}
		}
		`
		ssatest.CheckSyntaxFlow(t, code, `
		println(* #-> as $target)
		`, map[string][]string{
			"target": {"1", "2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}*/
