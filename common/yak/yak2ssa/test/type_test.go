package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
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
			ssa.NewSliceType(ssa.GetNumberType()))
	})

	t.Run("append-string", func(t *testing.T) {
		test.CheckType(t, `
a = ["a"]
target = append(a, "b", "c")`,
			ssa.NewSliceType(ssa.GetStringType()))
	})
}
