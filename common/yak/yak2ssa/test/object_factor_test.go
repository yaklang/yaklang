package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/ssatest"
)

func Test_ObjectFactor_Closure(t *testing.T) {

	t.Run("free value", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
			f = (arg) => {
				return arg["b"]
			}

			a = {
				"b": 1, 
			}
			println(f(a))
			`,
			Want: []string{
				"Function-main$1(make(map[string]number),1)",
			},
		})
	})

	t.Run("free value, object is free-value", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
			a = {}
			f = () => {
				return a.b
			}
			println(f())
			`,
			Want: []string{
				"Function-main$1(make(map[any]any),Undefined-#2.b(valid))",
			},
		})
	})

	t.Run("free value, object self", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
			a = {
				"key": 1, 
				"get": () => a.key, 
			} 
			println(a.get())
			`,
			Want: []string{
				"Function-main$1(make(map[string]any),1)",
			},
		})
	})

	t.Run("side effect", func(t *testing.T) {
		test.CheckPrintlnValue(`
		f = (arg) => {
			arg["b"] = 1 
		} 
		a = {}
		f(a)
		println(a.b)
		`, []string{
			"side-effect(1, #0.b)",
		}, t)
	})

	t.Run("side effect, object is free-value", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
			a = {}
			f = () => {
				a.b = 1
			}
			println(a.b)
			f()
			println(a.b)
			`,
			Want: []string{
				"Undefined-#2.b(valid)",
				"side-effect(1, #4.b)",
			},
		})
	})

	t.Run("side effect, self", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
			a = {
				"key": 1, 
				"set": (i) => {a.key = i}
			}
			println(a.key)
			a.set(1)
			println(a.key)
			`,
			Want: []string{
				"1",
				"side-effect(Parameter-i, #7.key)",
			},
		})
	})
}

func Test_ObjectFactor_Type(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckFunctionReturnType(t, `
		f = () => {
			this = {
				"key": 1, 
				"get": () => 1, 
			}
			return this
		}
		target = f
		`,
			ssa.MapTypeKind,
		)
	})

	t.Run("normal, free-value", func(t *testing.T) {
		test.CheckFunctionReturnType(t, `
		f = () => {
			a = 1
			this = {
				"key": 1, 
				"get": () => a
			}
			return this
		}
		target = f
		`,
			ssa.MapTypeKind,
		)
	})

	t.Run("normal, side-effect", func(t *testing.T) {
		test.CheckFunctionReturnType(t, `
		f = () => {
			a = 1
			this = {
				"key": 1, 
				"set": (i) => {a = i},
			}
			return this
		}
		target = f
		`,
			ssa.MapTypeKind,
		)
	})

	t.Run("class, free value", func(t *testing.T) {
		test.CheckFunctionReturnType(t, `
		f = () => {
			this = {
				"key": 1, 
				"get": () => this.key
			}
			return this
		}
		target = f
		`,
			ssa.ClassBluePrintTypeKind,
		)
	})
	t.Run("class, side effect", func(t *testing.T) {
		test.CheckFunctionReturnType(t, `
		f = () => {
			a = 1
			this = {
				"key": 1, 
				"set": (i) => {this.key = i},
			}
			return this
		}
		target = f
		`,
			ssa.ClassBluePrintTypeKind,
		)
	})
	t.Run("class, full", func(t *testing.T) {
		test.CheckFunctionReturnType(t, `
		f = () => {
			a = 1
			this = {
				"key": 1, 
				"set": (i) => {this.key = i},
				"get": () => this.key
			}
			return this
		}
		target = f
		`,
			ssa.ClassBluePrintTypeKind,
		)
	})
}
func Test_ObjectFactor_SideEffect(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintlnValue(`
		f = () => {
			this = {
				"key": 1, 
				"set": (i) => {this.key = i},
			}
			return this
		}
		a = f()
		println(a.key)
		a.set(2)
		println(a.key)
		`, []string{
			"Undefined-#13.key(valid)",
			"side-effect(Parameter-i, #8.key)",
		}, t)
	})

	t.Run("two object ", func(t *testing.T) {
		test.CheckPrintlnValue(`
		f = () => {
			this = {
				"key": 1, 
				"set": (i) => {this.key = i},
			}
			return this
		}
		b = f()
		a = f()
		println(a.key)
		a.set(2)
		println(a.key)
		`, []string{
			"Undefined-#15.key(valid)",
			"side-effect(Parameter-i, #8.key)",
		}, t)
	})

	t.Run("two object 2", func(t *testing.T) {
		test.CheckPrintlnValue(`
		f = () => {
			this = {
				"key": 1, 
				"set": (i) => {this.key = i},
			}
			return this
		}
		a = f()
		b = f()
		println(a.key)
		a.set(2)
		println(a.key)
		`, []string{
			"Undefined-#13.key(valid)",
			"side-effect(Parameter-i, #8.key)",
		}, t)
	})

}

func Test_ObjectFactor_FreeValue(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
		f = ()=> {
			this = {
				"key": 1,
				"get": () => this.key
			}
			return this
		}
		a = f()

		target = a.get()
		println(target)

		`,
			Want: []string{
				"Undefined-#12.get(valid)(Function-main$1(),Undefined-#12.key(valid))",
			},
		})
	})

	t.Run("two object", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
		f = ()=> {
			this = {
				"key": 1,
				"get": () => this.key
			}
			return this
		}

		a = f()
		b = f()

		target = b.get()
		println(target)

		`,
			Want: []string{
				"Undefined-#14.get(valid)(Function-main$1(),Undefined-#14.key(valid))",
			},
		})
	})

	t.Run("two object 2", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
		f = ()=> {
			this = {
				"key": 1,
				"get": () => this.key
			}
			return this
		}
		a = f()
		b = f()

		target = a.get()
		println(target)

		`,
			Want: []string{
				"Undefined-#12.get(valid)(Function-main$1(),Undefined-#12.key(valid))",
			},
		})
	})
}

func Test_ObjectFactor_ALL(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckPrintf(t, test.TestCase{
			Code: `
			f= () => {
				this = {
					"key": 1, 
					"get": () => this.key,
					"set": (i) => {this.key = i},
				}
				return this
			}

			a = f()
			a.set(2)
			println(a.get())

			b = f()
			b.set(2)
			println(b.get())


			b.set(3)
			println(a.get())

			a.set(3)
			println(b.get())
			`,
			Want: []string{
				"Undefined-#19.get(valid)(Function-main$1(),side-effect(Parameter-i, #14.key))",
				"Undefined-#32.get(valid)(Function-main$1(),side-effect(Parameter-i, #14.key))",
				"Undefined-#19.get(valid)(Function-main$1(),side-effect(Parameter-i, #14.key))",
				"Undefined-#32.get(valid)(Function-main$1(),side-effect(Parameter-i, #14.key))",
			},
		})
	})
}

func Test_Object_Assign(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckParameter(t, test.TestCase{
			Code: `
		target = func(https, request) {
			payloads = []
			for payload in payloads {
				rsp, err := payload.Fuzz(payload).ExecFirst()
				if err != nil {
					info("FAILED: %v" % err)
					continue
				}
				a = rsp.RequestRaw
				b = rsp.ResponseRaw
			}
		}
		`,
			Want: []string{"https", "request"},
		})
	})

	t.Run("normal 2", func(t *testing.T) {
		test.CheckError(t, test.TestCase{
			Code: `
		freq = fuzz.HTTPRequest("", fuzz.https(true))~
		params = freq.GetCommonParams()
		for i in params {
			rsp, err := i.Fuzz(payload).ExecFirst()
			if err != nil {
				info("FAILED: %v" % err)
				continue
			}
			a = rsp.RequestRaw
			b = rsp.ResponseRaw
		}
		`,
			Want: []string{
				ssa.ValueUndefined("info"),
				ssa.ValueUndefined("payload"),
			},
		})
	})
}
