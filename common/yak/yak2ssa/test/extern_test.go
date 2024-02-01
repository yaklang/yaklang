package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func TestExternLibInClosure(t *testing.T) {
	test := assert.New(t)
	prog, err := ssaapi.Parse(`
	a = () => {
		lib.method()
	}
	`,
		ssaapi.WithExternLib("lib", map[string]any{
			"method": func() {},
		}),
	)
	test.Nil(err)
	libVariables := prog.Ref("lib").ShowWithSource()
	// TODO: handler this
	// test.Equal(1, len(libVariables))
	test.NotEqual(0, len(libVariables))
	libVariable := libVariables[0]

	test.False(libVariable.IsParameter())
	test.True(libVariable.IsExternLib())
}

func TestExternValue(t *testing.T) {

	check := func(t *testing.T, tc TestCase, checkFunc func(*testing.T, *ssaapi.Program)) {
		tc.ExternValue = map[string]any{
			"println": func(v ...any) {},
		}
		tc.Check = checkFunc
		CheckTestCase(t, tc)
	}
	checkIsFunction := func(t *testing.T, tc TestCase) {
		check(t, tc,
			func(t *testing.T, prog *ssaapi.Program) {
				ps := prog.Ref("target").ShowWithSource()
				if len(
					ps.Filter(func(v *ssaapi.Value) bool {
						return !v.IsFunction()
					}),
				) != 0 {
					t.Fatalf("println should all is function")
				}
			},
		)
	}

	t.Run("use normal", func(t *testing.T) {
		checkIsFunction(t, TestCase{
			code: `
			target = println
			`,
		})
	})

	t.Run("use in loop", func(t *testing.T) {
		checkIsFunction(t, TestCase{
			code: `
			for i=0; i<10;i++{
				target = println
			}
			`,
		})
	})

	t.Run("use in closure", func(t *testing.T) {
		checkIsFunction(t, TestCase{
			code: `
			f = () => {
				target = println
			}
			`,
		})
	})

	t.Run("use in closure can capture", func(t *testing.T) {
		checkIsFunction(t, TestCase{
			code: `
			target = println
			f = () => {
				target = println
			}
			`,
		})
	})

	checkIsCover := func(t *testing.T, tc TestCase) {
		check(t, tc,
			func(t *testing.T, p *ssaapi.Program) {
				ps := p.Ref("target").ShowWithSource()
				if len(
					ps.Filter(func(v *ssaapi.Value) bool {
						return v.String() != "1"
					}),
				) != 0 {
					t.Fatalf("println should all is 1")
				}
			},
		)
	}

	t.Run("cover normal", func(t *testing.T) {
		checkIsCover(t, TestCase{
			code: `
			println = 1
			target = println
			`,
		})
	})

	t.Run("cover syntax block", func(t *testing.T) {
		checkIsFunction(t, TestCase{
			code: `
			{
				println = 1
			}
			target = println // this is function
			`,
		})
	})

	t.Run("cover in loop", func(t *testing.T) {
		checkIsCover(t, TestCase{
			code: `
			println = 1
			for i=0; i<10;i++{
				target = println
			}
			`,
		})
	})
}
