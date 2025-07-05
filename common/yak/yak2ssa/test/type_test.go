package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestYaklangBasic_Foreach(t *testing.T) {
	t.Run("for each with chan", func(t *testing.T) {
		test.CheckTypeKind(t, `
		ch = make(chan int)

		for i in ch { 
			_ = i 
			target = i
		}
		`,
			ssa.NumberTypeKind)
	})

	t.Run("for each with list", func(t *testing.T) {
		test.CheckTypeKind(t, `
		ch = make([]int, 3)

		for i in ch { 
			_ = i 
			target = i
		}
		`,
			ssa.NumberTypeKind)
	})
}

func TestYaklangType_Loop(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckTypeKind(t, `
		num = make([]int, 3)
		for i=0; i < 3; i++ {
			target = num[i]
		}
		`,
			ssa.NumberTypeKind)
	})
}

func TestYaklangType_BuildInMethod(t *testing.T) {
	t.Run("slice", func(t *testing.T) {
		test.CheckTypeKind(t, `
		s = make([]int, 3)
		target = s.Len()
		`,
			ssa.NumberTypeKind)
	})

	t.Run("loop", func(t *testing.T) {
		test.CheckTypeKind(t, `
		a = make([]int, 3)
		for i=0; i<3; i++ {
			target = a.Len()
		}`,
			ssa.NumberTypeKind)
	})
}

func TestYaklangType_FreeValue(t *testing.T) {
	t.Run("in closure", func(t *testing.T) {
		test.CheckTypeKind(t, `
		m = make(map[string]string)
		() => {
			target = m.Len()
		}
		`, ssa.NumberTypeKind)
	})

	t.Run("in sub closure", func(t *testing.T) {
		test.CheckTypeKind(t, `
		m = make(map[string]string)
		() => {
			() => {
				target = m.Len()
			}
		}
		`, ssa.NumberTypeKind)
	})

	t.Run("in loop", func(t *testing.T) {
		test.CheckTypeKind(t, `
		m = make(map[string]string)
		() => {
			for i=0; i<10; i++ {
				target = m.Len()
			}
		}
		`, ssa.NumberTypeKind)
	})
}

func TestYaklangType_Object(t *testing.T) {
	t.Run("map, but not found", func(t *testing.T) {
		test.CheckTypeKind(t, `
		m = make(map[string]string)
		target = m["key"]
		`, ssa.StringTypeKind)
	})

	t.Run("map, can found", func(t *testing.T) {
		test.CheckTypeKind(t, `
		m = make(map[string]any)
		m["key"] = 1
		target = m["key"]
		`, ssa.NumberTypeKind)
	})

	t.Run("map, not found, pass function", func(t *testing.T) {
		test.CheckTypeKind(t, `
		f = () => {
			m = make(map[string]string)
			return m
		}
		m = f() 
		target = m["key"]
		`, ssa.StringTypeKind)
	})

	t.Run("map, can found, pass function", func(t *testing.T) {
		test.CheckTypeKind(t, `
		f = () => {
			m = make(map[string]any)
			m["key"] = 1
			return m
		}
		m = f() 
		target = m["key"]
		`, ssa.NumberTypeKind)
	})

	t.Run("map, just create and return", func(t *testing.T) {
		test.CheckTypeKind(t, `
		f = () => ({
			"key": 1
		})
		m = f()
		target = m["key"]
		`, ssa.NumberTypeKind)
	})
}

func TestYaklangGenericFunc(t *testing.T) {
	t.Run("append-int", func(t *testing.T) {
		test.CheckType(t, `
a = [1]
target = append(a, 2, 3)
cache = append(a, 4, 5)`,
			ssa.NewSliceType(ssa.CreateNumberType()))
	})

	t.Run("append-bytes", func(t *testing.T) {
		test.CheckType(t, `
a = b"asd"
b = b"qwe"
target = append(a, b...)`,
			ssa.CreateBytesType())
	})

	t.Run("append-bytes-with-bytes-fallback", func(t *testing.T) {
		test.CheckType(t, `
		a = b"asd"
		target = append(a, b"qwe", b"zxc")`,
			ssa.NewSliceType(ssa.CreateAnyType()))
	})

	t.Run("append-string", func(t *testing.T) {
		test.CheckType(t, `
a = ["a"]
target = append(a, "b", "c")`,
			ssa.NewSliceType(ssa.CreateStringType()))
	})

	t.Run("Keys-string", func(t *testing.T) {
		test.CheckType(t, `
a = {"a":1, "b":2}
target = Keys(a)`,
			ssa.NewSliceType(ssa.CreateStringType()),
			ssaapi.WithExternValue(map[string]any{
				"Keys": func(i interface{}) interface{} {
					// ...
					return nil
				},
			}),
			ssaapi.WithExternBuildValueHandler("Keys", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
				// Keys(map[T]U) []T
				typ := ssa.NewFunctionTypeDefine(id, []ssa.Type{ssa.NewMapType(ssa.TypeT, ssa.TypeU)}, []ssa.Type{ssa.NewSliceType(ssa.TypeT)}, false)
				f := ssa.NewFunctionWithType(id, typ)
				f.SetGeneric(true)
				f.SetRange(b.CurrentRange)
				return f
			}),
		)
	})

	t.Run("Complex-wrap-type", func(t *testing.T) {
		test.CheckType(t, `
a = make([]chan string, 0)
a = append(a, make(chan string))
a = append(a, make(chan string))
target = Test(a)`,
			ssa.NewSliceType(ssa.CreateStringType()),
			ssaapi.WithExternValue(map[string]any{
				"Test": func(i interface{}) interface{} {
					// ...
					return nil
				},
			}),
			ssaapi.WithExternBuildValueHandler("Test", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
				// Test([]chan T) []T
				typ := ssa.NewFunctionTypeDefine(id, []ssa.Type{ssa.NewSliceType(ssa.NewChanType(ssa.TypeT))}, []ssa.Type{ssa.NewSliceType(ssa.TypeT)}, false)
				f := ssa.NewFunctionWithType(id, typ)
				f.SetGeneric(true)
				f.SetRange(b.CurrentRange)
				return f
			}),
		)
	})

	t.Run("OrType", func(t *testing.T) {
		test.CheckType(t, `
a = {"a":1, "b":2}
target = Test(a, func(i) { return int(i) })
`,
			ssa.NewSliceType(ssa.CreateNumberType()),
			ssaapi.WithExternValue(map[string]any{
				"Test": func(i interface{}, fc func(i interface{}) interface{}) interface{} {
					// func(Or([]T | Map[U]T), func(T) K) []K
					return nil
				},
			}),
			ssaapi.WithExternBuildValueHandler("Test", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
				// Test(Or([]T | Map[U]T), func(T) K) []K
				typ := ssa.NewFunctionTypeDefine(id, []ssa.Type{
					ssa.NewOrType(
						ssa.NewSliceType(ssa.TypeT),
						ssa.NewMapType(ssa.TypeU, ssa.TypeT),
					),
					ssa.NewFunctionTypeDefine("anonymous",
						[]ssa.Type{ssa.TypeT},
						[]ssa.Type{ssa.TypeK},
						false),
				}, []ssa.Type{ssa.NewSliceType(ssa.TypeK)}, false)
				f := ssa.NewFunctionWithType(id, typ)
				f.SetGeneric(true)
				f.SetRange(b.CurrentRange)
				return f
			}),
		)
	})
}

func TestYaklangTypeInference(t *testing.T) {
	t.Run("function-parameter", func(t *testing.T) {
		test.CheckTypeEx(t, `
openapi.flowHandler(flow => {
	target = flow
})
		`,
			func(p *ssaapi.Program) *ssaapi.Type {
				return p.GetType("schema.HTTPFlow")
			})
	})
}
