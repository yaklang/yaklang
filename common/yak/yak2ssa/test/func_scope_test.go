package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestClosure_FreeValue_Value(t *testing.T) {

	t.Run("normal function", func(t *testing.T) {
		test.CheckPrintlnValue(`
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
		test.CheckPrintlnValue(`
		f = () => {
			println(a)
		}
		`, []string{
			"FreeValue-a",
		}, t)
	})

	t.Run("closure function, only free-value, can capture", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a  = 1
		f = () => {
			println(a)
		}
		`, []string{
			"FreeValue-a",
		}, t)
	})

	t.Run("closure function, capture variable but in this function", func(t *testing.T) {
		test.CheckPrintlnValue(`
		f = () => {
			a = 1
			{
				println(a)
			}
		}`, []string{
			"1",
		}, t)
	})

	t.Run("closure function, can capture parent-variable, use local variable, not same", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		f = ()=>{
			a := 1
			{
				println(a)
			}
		}`, []string{"1"}, t)
	})

	t.Run("closure function, side-effect, con't capture", func(t *testing.T) {
		test.CheckPrintlnValue(`
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
		test.CheckPrintlnValue(`
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

	t.Run("FreeValue self", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = () => {
			a = 2
		}
		`, []string{}, t)
	})
}

func TestClosure_FreeValue_Function(t *testing.T) {
	t.Run("func capture value", func(t *testing.T) {
		test.CheckFreeValue(t, test.TestCase{
			Code: `
		a = 1
		f = () => {
			b = a
		}
		target = f
		`,
			Want: []string{"a"},
		})
	})

	t.Run("member capture value", func(t *testing.T) {
		test.CheckFreeValue(t, test.TestCase{
			Code: `
		a = 1
		b = {
			"get": () => a
		}

		target = b.get 
		`,
			Want: []string{"a"},
		})
	})

	t.Run("func capture member", func(t *testing.T) {
		test.CheckParameterMember(t, test.TestCase{
			Code: ` 
			a = {
				"key": 1,
			}
			f = () => {
				b = a.key
			}
			target = f
			`,
			Want: []string{
				"freeValue-a.key",
			},
		})
	})

	t.Run("member capture member", func(t *testing.T) {
		test.CheckParameterMember(t, test.TestCase{
			Code: `
			a = {
				"key": 1, 
			}
			b = {
				"get": () => a.key
			}
			target = b.get
			`,
			Want: []string{
				"freeValue-a.key",
			},
		})
	})

	t.Run("member capture member, self", func(t *testing.T) {
		test.CheckParameterMember(t, test.TestCase{
			Code: `
			a = {
				"key": 1, 
				"get": () => a.key
			}
			target = a.get
			`,
			Want: []string{
				"parameter[0].key",
			},
		})
	})
}

func TestClosure_Mask(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		test.CheckMask(t, test.TestCase{
			Code: `
			a = 1
			f = () => {
				a = 2
			}
			target = a
			`,
			Want: []string{
				"2",
			},
		})
	})

	t.Run("closure function, freeValue and Mask", func(t *testing.T) {
		test.CheckMask(t, test.TestCase{
			Code: `
			a = 1
			f = () => {
				a = a + 2
			}
			target = a
			`,
			Want: []string{"add(FreeValue-a, 2)"},
		})
	})

	// t.Run("object member", func(t *testing.T) {
	// 	test.CheckMask(t, test.TestCase{
	// 		Code: `
	// 		a = {
	// 			"key": 1,
	// 		}
	// 		f = () => {
	// 			a.key = 2
	// 		}
	// 		target = a.key
	// 		`,
	// 		Want: []string{"2"},
	// 	})
	// })

	// t.Run("object member, not found", func(t *testing.T) {
	// 	test.CheckMask(t, test.TestCase{
	// 		Code: `
	// 	a = {}
	// 	f = () => {
	// 		a.key = 2
	// 	}
	// 	target = a.key
	// 	`,
	// 		Want: []string{"2"},
	// 	})
	// })

	// t.Run("object member, self", func(t *testing.T) {
	// 	test.CheckMask(t, test.TestCase{
	// 		Code: `
	// 		a = {
	// 			"key": 1,
	// 			"set": (i) => {a.key = i}
	// 		}
	// 		target = a.key
	// 		`,
	// 		Want: []string{},
	// 	})
	// })
}

func TestClosure_SideEffect(t *testing.T) {

	t.Run("function modify value", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 0 
		b = () => {
			a = 1
		}

		if c {
			b() // a = 1
		}
		println(a) // phi 1, 0
		`, []string{
			"phi(a)[side-effect(1, a),0]",
		}, t)
	})

	t.Run("func not modify value", func(t *testing.T) {
		test.CheckPrintlnValue(`
		f  = () =>{}
		{
			a = 1
			f = () => {
				a = 2
			}
			println(a) // 1
			f()
			println(a) // side-effect(2, a)
		}
		a = 1
		println(a) // 1
		f() 
		println(a) // side-effect(2, a)
		`, []string{
			"1", "side-effect(2, a)",
			"1", "side-effect(2, a)",
		}, t)
	})

	t.Run("object member modify value", func(t *testing.T) {
		test.CheckPrintlnValue(`
		var b
		get = () => ({
			"change": i=>{b=i}	
		})
		a = get() 
		a.change("c")
		println(b)
		`, []string{
			"side-effect(Parameter-i, b)",
		}, t)
	})

	t.Run("function modify object member", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a =  {
			"key": 1,
		}
		f = (i) => {
			a.key = i
		}

		println(a.key) // 1
		f(2) 
		println(a.key) // parameter-i
		`, []string{
			"1",
			"side-effect(Parameter-i, a.key)",
		}, t)
	})

	t.Run("function modify object member, not found", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a =  {}
		f = (i) => {
			a.key = i
		}

		println(a.key) // undefined
		f(2) 
		println(a.key) // parameter-i
		`, []string{
			"Undefined-a.key(valid)",
			"side-effect(Parameter-i, a.key)",
		}, t)
	})

	t.Run("member modify member", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = {
			"key": 1, 
		}
		b = {
			"change": (i)=>{
				a.key = i
			}
		}
		println(a.key)
		b.change(2)
		println(a.key)
		`, []string{
			"1",
			"side-effect(Parameter-i, a.key)",
		}, t)
	})

	t.Run("object modify self", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = {
			"key": 1,
			"add": (i) => {a.key = i},
		}
		println(a.key)
		a.add(2)
		println(a.key)
		`, []string{
			"1",
			"side-effect(Parameter-i, a.key)",
		}, t)
	})
}

func TestClosure_HasSideEffect(t *testing.T) {
	t.Run("exist side-effect", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1 
		() => {
			a = 3
		}()
		println(a)
	`, []string{
			"side-effect(3, a)",
		}, t)
	})

	t.Run("local variable not side-effect", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1 
		() => {
			a := 3
		}()
		println(a)
	`, []string{
			"1",
		}, t)
	})

}
