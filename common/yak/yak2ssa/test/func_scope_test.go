package test

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestClosureFreeValueScope(t *testing.T) {

	t.Run("normal function", func(t *testing.T) {
		checkPrintlnValue(`
		func a(){
			a = 1
			println(a)
		}
		a()
		`, []string{
			"1",
		}, t)
	})

	t.Run("closure function, only free-value, con't capture", func(t *testing.T) {
		checkPrintlnValue(`
		f = () => {
			println(a)
		}
		`, []string{
			"FreeValue-a",
		}, t)
	})

	t.Run("closure function, only free-value, can capture", func(t *testing.T) {
		checkPrintlnValue(`
		a  = 1
		f = () => {
			println(a)
		}
		`, []string{
			"FreeValue-a",
		}, t)
	})

	t.Run("closure function, capture variable but in this function", func(t *testing.T) {
		checkPrintlnValue(`
		f = () => {
			a = 1
			{
				println(a)
			}
		}`, []string{
			"1",
		}, t)
	})

	t.Run("closure function, can capture parent-variable, same", func(t *testing.T) {
		checkPrintlnValue(`
		a = 1
		f = ()=>{
			a = 1
			{
				println(a)
			}
		}`, []string{"FreeValue-a"}, t)
	})

	t.Run("closure function, can capture parent-variable, use local variable, not same", func(t *testing.T) {
		checkPrintlnValue(`
		a = 1
		f = ()=>{
			a := 1
			{
				println(a)
			}
		}`, []string{"1"}, t)
	})

	t.Run("closure function, side-effect, con't capture", func(t *testing.T) {
		checkPrintlnValue(`
		f = () => {
			a = 2
			println(a)
		}
		println(a)
		`, []string{
			"2", "Undefined-a",
		}, t)
	})

	t.Run("closure function, side-effect, can capture", func(t *testing.T) {
		checkPrintlnValue(`
		a = 1
		f = () => {
			a = 2
			println(a)
		}
		println(a)
		`, []string{
			"2", "1",
		}, t)
	})
}

func TestClosureMask(t *testing.T) {
	check := func(t *testing.T, tc TestCase) {
		tc.Check = func(t *testing.T, p *ssaapi.Program) {
			test := assert.New(t)

			targets := p.Ref("target").ShowWithSource()
			test.Len(targets, 1)

			target := targets[0]

			v := ssaapi.GetBareNode(target)
			test.NotNil(v)

			test.Equal("1", v.String())

			maskV, ok := v.(ssa.Maskable)
			test.True(ok)

			maskValues := maskV.GetMask()
			log.Infof("mask values: %s", maskValues)

			test.Equal(tc.want, lo.Map(maskValues, func(v ssa.Value, _ int) string { return ssa.LineDisasm(v) }))
		}
		CheckTestCase(t, tc)
	}

	t.Run("normal", func(t *testing.T) {
		check(t, TestCase{
			code: `
			a = 1
			f = () => {
				a = 2
			}
			target = a
			`,
			want: []string{
				"2",
			},
		})
	})

	t.Run("closure function, freeValue and Mask", func(t *testing.T) {
		check(t, TestCase{
			code: `
			a = 1
			f = () => {
				a = a + 2
			}
			target = a
			`,
			want: []string{"add(FreeValue-a, 2)"},
		})
	})
}
