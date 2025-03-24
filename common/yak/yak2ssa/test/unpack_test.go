package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestUnpack_Line(t *testing.T) {

	t.Run("simple", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a, b = 1, 2
		println(a)
		println(b)
		`, []string{
			"1", "2",
		}, t)
	})

	t.Run("normal", func(t *testing.T) {
		test.CheckPrintlnValue(`
		m = [1, 2]
		a, b = m
		println(a)
		println(b)
		`, []string{
			"1", "2",
		}, t)
	})

	t.Run("normal function", func(t *testing.T) {
		test.CheckPrintlnValue(`
		f = () => {
			return 1, 2
		}
		a, b = f()
		println(a)
		println(b)
		`, []string{
			"Undefined-a(valid)",
			"Undefined-b(valid)",
		}, t)
	})

	t.Run("normal extern function ", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
			a, b = f()
			println(a)
			println(b)
			`,
			Want: []string{
				"Undefined-a(valid)",
				"Undefined-b(valid)",
			},
			ExternValue: map[string]any{
				"f": func() (int, int) { return 1, 2 },
			},
		})
	})

	t.Run("check variable ", func(t *testing.T) {
		test.CheckTestCase(t, test.TestCase{
			Code: `
		f = () => {
			return 1, 2
		}
		c = f()
		a, b = c
		`,
			Want: []string{
				"Undefined-#4[0](valid)",
				"Undefined-#4[1](valid)",
			},
			Check: func(p *ssaapi.Program, s []string) {
				as := p.Ref("a").ShowWithSource()
				require.Equal(t, 1, len(as), "a should only 1")
				a := as[0]

				callVariable := a.GetAllVariables()
				for i, v := range callVariable {
					log.Infof("call variable %s: %v", i, v)
				}
			},
		})
	})
}
