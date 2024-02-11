package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestUnpack_Line(t *testing.T) {

	t.Run("simple", func(t *testing.T) {
		checkPrintlnValue(`
		a, b = 1, 2
		println(a)
		println(b)
		`, []string{
			"1", "2",
		}, t)
	})

	t.Run("normal", func(t *testing.T) {
		checkPrintlnValue(`
		m = [1, 2]
		a, b = m
		println(a)
		println(b)
		`, []string{
			"1", "2",
		}, t)
	})

	t.Run("normal function", func(t *testing.T) {
		checkPrintlnValue(`
		f = () => {
			return 1, 2
		}
		a, b = f()
		println(a)
		println(b)
		`, []string{
			"Undefined-#4[0](valid)",
			"Undefined-#4[1](valid)",
		}, t)
	})

	t.Run("normal extern function ", func(t *testing.T) {
		CheckPrintf(t, TestCase{
			code: `
			a, b = f()
			println(a)
			println(b)
			`,
			want: []string{
				"Undefined-#0[0](valid)",
				"Undefined-#0[1](valid)",
			},
			ExternValue: map[string]any{
				"f": func() (int, int) { return 1, 2 },
			},
		})
	})

	t.Run("check variable ", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
		f = () => {
			return 1, 2
		}
		c = f()
		a, b = c
		`,
			want: []string{
				"Undefined-#4[0](valid)",
				"Undefined-#4[1](valid)",
			},
			Check: func(t *testing.T, p *ssaapi.Program, s []string) {
				test := assert.New(t)
				as := p.Ref("a").ShowWithSource()
				test.Equal(1, len(as), "a should only 1")
				a := as[0]

				call := ssaapi.GetBareNode(a)
				callVariable := call.GetAllVariables()
				for i, v := range callVariable {
					log.Infof("call variable %s: %v", i, v)
				}
			},
		})
	})
}
