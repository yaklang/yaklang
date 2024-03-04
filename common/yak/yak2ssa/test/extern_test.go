package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	checkIsFunction := func(t *testing.T, tc TestCase) {
		tc.ExternValue = map[string]any{
			"println": func(v ...any) {},
		}
		tc.Check = func(prog *ssaapi.Program, want []string) {
			ps := prog.Ref("target").ShowWithSource()
			require.NotEqual(t, 0, len(ps), "target should has value")
			require.Equal(t, 0,
				len(ps.Filter(func(v *ssaapi.Value) bool {
					return !v.IsFunction()
				})),
				"target should all is function",
			)
		}
		_CheckTestCase(t, tc)
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
		tc.ExternValue = map[string]any{
			"println": func(v ...any) {},
		}
		tc.Check = func(p *ssaapi.Program, s []string) {
			vs := p.Ref("target")
			// test.Len(vs, 0, "target should has value")
			require.NotEqual(t, 0, len(vs), "target should has value")
			require.NotEqual(t, 0, len(vs.Filter(func(v *ssaapi.Value) bool {
				return v.String() != "1"
			})),
				"target should all is 1",
			)
		}
	}

	t.Run("cover normal", func(t *testing.T) {
		checkIsCover(t, TestCase{
			code: `
			println = 1
			target = println
			`,
		})
	})

	t.Run("cover check in syntax block", func(t *testing.T) {
		checkIsCover(t, TestCase{
			code: `
			println = 1
			{
				target = println
			}
			`,
		})
	})

	t.Run("cover assign in syntax block", func(t *testing.T) {
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
			for i in 10{
				target = println
			}
			`,
		})
	})
}

func TestExternLib(t *testing.T) {
	checkIsExtern := func(t *testing.T, tc TestCase) {
		tc.ExternLib = map[string]map[string]any{
			"lib": map[string]any{
				"method": func() {},
			},
		}
		tc.Check = func(p *ssaapi.Program, s []string) {
			require.NotEqual(t, 0, p.Ref("lib").Filter(func(v *ssaapi.Value) bool {
				return !v.IsExternLib()
			}), "lib shoud all is externLib")
			require.NotEqual(t, 0, p.Ref("target").Filter(func(v *ssaapi.Value) bool {
				return !v.IsFunction()
			}), "println should all is function")
		}
		_CheckTestCase(t, tc)
	}

	t.Run("use normal", func(t *testing.T) {
		checkIsExtern(t, TestCase{
			code: `
			target = lib.method
			`,
		})
	})

	t.Run("use in loop", func(t *testing.T) {
		checkIsExtern(t, TestCase{
			code: `
			for i in 10 {
				target = lib.method
			}
			`,
		})
	})

	t.Run("use in closure", func(t *testing.T) {
		checkIsExtern(t, TestCase{
			code: `
			f = () => {
				target = lib.method
			}
			`,
		})
	})

	t.Run("use in closure, can capture method", func(t *testing.T) {
		checkIsExtern(t, TestCase{
			code: `
				target = lib.method
				f = () => {
					target = lib.method
				}
				`,
		})
	})

	t.Run("use in closure, can capture lib", func(t *testing.T) {
		checkIsExtern(t, TestCase{
			code: `
				b = lib
				f = () => {
					target = lib.method
				}
				`,
		})
	})

	checkIsCover := func(t *testing.T, tc TestCase) {
		tc.ExternLib = map[string]map[string]any{
			"lib": map[string]any{
				"method": func() {},
			},
		}
		tc.Check = func(p *ssaapi.Program, s []string) {
			vs := p.Ref("target")
			require.NotEqual(t, 0, len(vs), "target should has value")
			require.NotEqual(t, 0, vs.Filter(func(v *ssaapi.Value) bool {
				return v.String() != "1"
			}), "target should all is 1")
		}
	}

	t.Run("cover normal  ", func(t *testing.T) {
		checkIsCover(t, TestCase{
			code: `
			lib.method = 1
			target = lib.method
			`,
		})
	})

	t.Run("cover assign in syntax block", func(t *testing.T) {
		checkIsExtern(t, TestCase{
			code: `
			{
				lib.method = 1
			}
			target = lib.method
			`,
		})
	})

	t.Run("cover check in syntax block", func(t *testing.T) {
		checkIsCover(t, TestCase{
			code: `
			lib.method = 1
			{
				target = lib.method
			}
			`,
		})
	})

	t.Run("cover check in loop", func(t *testing.T) {
		checkIsCover(t, TestCase{
			code: `
			lib.method = 1
			for i in 10{
				target = lib.method
			}
			`,
		})
	})
}

func TestExternRef(t *testing.T) {
	check := func(t *testing.T, tc TestCase) {
		tc.ExternValue = map[string]any{
			"println": func(v ...any) {},
		}
		tc.ExternLib = map[string]map[string]any{
			"lib": map[string]any{
				"method": func() {},
			},
		}
		_CheckTestCase(t, tc)
	}

	t.Run("extern value", func(t *testing.T) {
		check(t, TestCase{
			code: `
			println("a")
			println("a")
			println("a")
			println("a")
			`,
			Check: func(p *ssaapi.Program, want []string) {
				printlns := p.Ref("println")
				require.Len(t, printlns, 1)

				println := printlns[0]
				require.True(t, println.IsFunction())

				printlnCaller := println.GetUsers()
				require.Len(t, printlnCaller, 4)
			},
		})
	})

	t.Run("extern lib", func(t *testing.T) {
		check(t, TestCase{
			code: `
			lib.method()
			lib.method()
			lib.method()
			lib.method()
			lib.method()
			lib.method()
			lib.method()
			`,
			Check: func(p *ssaapi.Program, w []string) {
				libs := p.Ref("lib")
				require.Len(t, libs, 1)

				lib := libs[0]
				require.True(t, lib.IsExternLib())

				methods := lib.GetOperands()
				require.Len(t, methods, 1)

				method := methods[0]
				require.True(t, method.IsFunction())

				methodCaller := method.GetUsers()
				require.Len(t, methodCaller, 7)
			},
		})
	})

}
