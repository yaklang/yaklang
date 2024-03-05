package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

func Test_ObjectFactor_Closure(t *testing.T) {

	t.Run("free value", func(t *testing.T) {
		checkPrintf(t, TestCase{
			code: `
			f = (arg) => {
				return arg["b"]
			}

			a = {
				"b": 1, 
			}
			println(f(a))
			`,
			want: []string{
				"Function-main$1(make(map[string]number),1)",
			},
		})
	})

	t.Run("free value, object is free-value", func(t *testing.T) {
		checkPrintf(t, TestCase{
			code: `
			a = {}
			f = () => {
				return a.b
			}
			println(f())
			`,
			want: []string{
				"Function-main$1(make(map[any]any),Undefined-#2.b(valid))",
			},
		})
	})

	t.Run("free value, object self", func(t *testing.T) {
		checkPrintf(t, TestCase{
			code: `
			a = {
				"key": 1, 
				"get": () => a.key, 
			} 
			println(a.get())
			`,
			want: []string{
				"Function-main$1(make(map[string]any),1)",
			},
		})
	})

	t.Run("side effect", func(t *testing.T) {
		checkPrintlnValue(`
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
		checkPrintf(t, TestCase{
			code: `
			a = {}
			f = () => {
				a.b = 1
			}
			println(a.b)
			f()
			println(a.b)
			`,
			want: []string{
				"Undefined-#2.b(valid)",
				"side-effect(1, #4.b)",
			},
		})
	})

	t.Run("side effect, self", func(t *testing.T) {
		checkPrintf(t, TestCase{
			code: `
			a = {
				"key": 1, 
				"set": (i) => {a.key = i}
			}
			println(a.key)
			a.set(1)
			println(a.key)
			`,
			want: []string{
				"1",
				"side-effect(Parameter-i, #7.key)",
			},
		})
	})
}

func Test_ObjectFactor_Type(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		checkFunctionReturnType(t, `
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
		checkFunctionReturnType(t, `
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
		checkFunctionReturnType(t, `
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
		checkFunctionReturnType(t, `
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
		checkFunctionReturnType(t, `
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
		checkFunctionReturnType(t, `
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
		checkPrintlnValue(`
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

}

func Test_ObjectFactor_FreeValue(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		checkPrintf(t, TestCase{
			code: `
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
			want: []string{
				"Undefined-#12.get(valid)(Function-main$1(),Undefined-#12.key(valid))",
			},
		})
	})
}
