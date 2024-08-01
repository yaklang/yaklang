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
		ssatest.Check(t, code,
			ssatest.CheckTopDef_Equal("a", []string{"1"},) ,
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}