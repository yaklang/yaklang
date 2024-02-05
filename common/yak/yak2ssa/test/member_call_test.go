package test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestMemberCall(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		checkPrintlnValue(`
		a = {}
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	t.Run("normal slice", func(t *testing.T) {
		checkPrintlnValue(`
		a = [] 
		a[0] = 1
		println(a[0])
		`, []string{"1"}, t)
	})

}

func TestMemberCallNegative(t *testing.T) {

	/// check v
	t.Run("expr is undefine, create before", func(t *testing.T) {
		checkPrintlnValue(`
		b = a
		println(a.b)
		`, []string{"Undefined-a.b"}, t)
	})

	t.Run("expr is undefine, create right-now", func(t *testing.T) {
		checkPrintlnValue(`
		println(a.b)
		`, []string{"Undefined-a.b"}, t)
	})

	t.Run("expr conn't be index", func(t *testing.T) {
		checkPrintlnValue(`
		a = 1
		println(a.b)
		`, []string{"1.b"}, t)
	})

	// in left
	t.Run("expr is undefine in left", func(t *testing.T) {
		checkPrintlnValue(`
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})
	t.Run("expr is undefine, create before, in left", func(t *testing.T) {
		checkPrintlnValue(`
		b = a
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	t.Run("expr is, conn't be index, in left", func(t *testing.T) {
		checkPrintlnValue(`
		a = 1
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	// expr = {}
	t.Run("expr is make", func(t *testing.T) {
		checkPrintlnValue(`
		a = {
			"A": 1,
		}

		println(a["A"])

		a["A"] = 2
		println(a["A"])
		`, []string{
			"1", "2",
		}, t)
	})

	// check key
	t.Run("expr normal, but undefine expr.key,", func(t *testing.T) {
		checkPrintlnValue(`
		v = {}
		println(v.key)
		`, []string{"make(map[any]any).key"}, t)
	})

	t.Run("expr normal, key is type", func(t *testing.T) {
		checkPrintlnValue(`
		v = "111"
		println(v[1])
		`, []string{
			`"111".1`,
		}, t,
		)
	})

}

func TestMemberCall_CheckField(t *testing.T) {
	t.Run("assign", func(t *testing.T) {
		checkPrintlnValue(`
		a = {} 
		if c {
			a.b = 1
		}
		println(a.b)
		`, []string{
			"phi(#2.b)[1,make(map[any]any).b]",
		}, t)
	})

	t.Run("read", func(t *testing.T) {
		CheckTestCase(t, TestCase{
			code: `
		a = {}
		if c {
			println(a.b)
		}
		println(a.b)
			`,
			Check: func(t *testing.T, p *ssaapi.Program) {
				test := assert.New(t)
				printlns := p.Ref("println").ShowWithSource()
				arg := printlns.GetUsers().Filter(func(v *ssaapi.Value) bool {
					return v.IsCall()
				}).Flat(func(v *ssaapi.Value) ssaapi.Values {
					return ssaapi.Values{v.GetOperand(1)}
				}).ShowWithSource()

				argUniqed := lo.UniqBy(arg, func(v *ssaapi.Value) int {
					return v.GetId()
				})

				test.Len(argUniqed, 1)
			},
		})
	})
}
