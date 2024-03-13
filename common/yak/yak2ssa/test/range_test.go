package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func Test_Yaklang_range(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		ssatest.CheckTestCase(t, ssatest.TestCase{
			Code: `
			a = 1
			{
				a := 2
			}
			println(a)
			{
				a = 3
			}
			println(a)
			`,
			Want: []string{},
			Check: func(prog *ssaapi.Program, t []string) {
				value := prog.GetFrontValueByOffset(9)
				// _ = value
				value.ShowWithSource()
			},
		})
	})

}
