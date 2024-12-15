package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
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
	checkIsFunction := func(t *testing.T, tc test.TestCase) {
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
		test.CheckTestCase(t, tc)
	}

	checkIsFunctionOrFreeValue := func(t *testing.T, tc test.TestCase) {
		tc.ExternValue = map[string]any{
			"println": func(v ...any) {},
		}
		tc.Check = func(prog *ssaapi.Program, want []string) {
			ps := prog.Ref("target").ShowWithSource()
			require.NotEqual(t, 0, len(ps), "target should has value")
			require.Equal(t, true, ps[0].IsFunction(), "target should all is function")
			require.Equal(t, true, ps[1].IsFreeValue(), "target should all is free value")
		}
		test.CheckTestCase(t, tc)
	}

	t.Run("use normal", func(t *testing.T) {
		checkIsFunction(t, test.TestCase{
			Code: `
			target = println
			`,
		})
	})

	t.Run("use in loop", func(t *testing.T) {
		checkIsFunction(t, test.TestCase{
			Code: `
			for i=0; i<10;i++{
				target = println
			}
			`,
		})
	})

	t.Run("use in closure", func(t *testing.T) {
		checkIsFunction(t, test.TestCase{
			Code: `
			f = () => {
				target = println
			}
			`,
		})
	})

	t.Run("use in closure can capture", func(t *testing.T) {
		// 目前捕获到的side-effect会格外生成一个freevalue
		checkIsFunctionOrFreeValue(t, test.TestCase{
			Code: `
			target = println
			f = () => {
				target = println
			}
			`,
		})
	})

	checkIsCover := func(t *testing.T, tc test.TestCase) {
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
		checkIsCover(t, test.TestCase{
			Code: `
			println = 1
			target = println
			`,
		})
	})

	t.Run("cover check in syntax block", func(t *testing.T) {
		checkIsCover(t, test.TestCase{
			Code: `
			println = 1
			{
				target = println
			}
			`,
		})
	})

	t.Run("cover assign in syntax block", func(t *testing.T) {
		checkIsFunction(t, test.TestCase{
			Code: `
			{
				println = 1
			}
			target = println // this is function
			`,
		})
	})

	t.Run("cover in loop", func(t *testing.T) {
		checkIsCover(t, test.TestCase{
			Code: `
			println = 1
			for i in 10{
				target = println
			}
			`,
		})
	})
}

func TestExternLib(t *testing.T) {
	checkIsExtern := func(t *testing.T, tc test.TestCase) {
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
		test.CheckTestCase(t, tc)
	}

	t.Run("use normal", func(t *testing.T) {
		checkIsExtern(t, test.TestCase{
			Code: `
			target = lib.method
			`,
		})
	})

	t.Run("use in loop", func(t *testing.T) {
		checkIsExtern(t, test.TestCase{
			Code: `
			for i in 10 {
				target = lib.method
			}
			`,
		})
	})

	t.Run("use in closure", func(t *testing.T) {
		checkIsExtern(t, test.TestCase{
			Code: `
			f = () => {
				target = lib.method
			}
			`,
		})
	})

	t.Run("use in closure, can capture method", func(t *testing.T) {
		checkIsExtern(t, test.TestCase{
			Code: `
				target = lib.method
				f = () => {
					target = lib.method
				}
				`,
		})
	})

	t.Run("use in closure, can capture lib", func(t *testing.T) {
		checkIsExtern(t, test.TestCase{
			Code: `
				b = lib
				f = () => {
					target = lib.method
				}
				`,
		})
	})

	checkIsCover := func(t *testing.T, tc test.TestCase) {
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
		checkIsCover(t, test.TestCase{
			Code: `
			lib.method = 1
			target = lib.method
			`,
		})
	})

	t.Run("cover assign in syntax block", func(t *testing.T) {
		checkIsExtern(t, test.TestCase{
			Code: `
			{
				lib.method = 1
			}
			target = lib.method
			`,
		})
	})

	t.Run("cover check in syntax block", func(t *testing.T) {
		checkIsCover(t, test.TestCase{
			Code: `
			lib.method = 1
			{
				target = lib.method
			}
			`,
		})
	})

	t.Run("cover check in loop", func(t *testing.T) {
		checkIsCover(t, test.TestCase{
			Code: `
			lib.method = 1
			for i in 10{
				target = lib.method
			}
			`,
		})
	})
}

func TestExternRef(t *testing.T) {
	check := func(t *testing.T, tc test.TestCase) {
		tc.ExternValue = map[string]any{
			"println": func(v ...any) {},
		}
		tc.ExternLib = map[string]map[string]any{
			"lib": map[string]any{
				"method": func() {},
			},
		}
		test.CheckTestCase(t, tc)
	}

	t.Run("extern value", func(t *testing.T) {
		check(t, test.TestCase{
			Code: `
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
		check(t, test.TestCase{
			Code: `
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
