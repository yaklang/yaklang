package test

import (
	"testing"
)

func TestClosure_FreeValue_Value(t *testing.T) {

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

	t.Run("FreeValue self", func(t *testing.T) {
		checkPrintlnValue(`
		a = () => {
			a = 2
		}
		`, []string{}, t)
	})
}

func TestClosure_FreeValue_Function(t *testing.T) {
	t.Run("func capture value", func(t *testing.T) {
		checkFreeValue(t, TestCase{
			code: `
		a = 1
		f = () => {
			b = a
		}
		target = f
		`,
			want: []string{"a"},
		})
	})

	t.Run("member capture value", func(t *testing.T) {
		checkFreeValue(t, TestCase{
			code: `
		a = 1
		b = {
			"get": () => a
		}

		target = b.get 
		`,
			want: []string{"a"},
		})
	})

	t.Run("func capture member", func(t *testing.T) {
		checkParameter(t, TestCase{
			code: ` 
			a = {
				"key": 1,
			}
			f = () => {
				b = a.key
			}
			target = f
			`,
			want: []string{
				"a",
				"#5.key",
			},
		})
	})

	t.Run("member capture member", func(t *testing.T) {
		checkParameter(t, TestCase{
			code: `
			a = {
				"key": 1, 
			}
			b = {
				"get": () => a.key
			}
			target = b.get
			`,
			want: []string{"a", "#6.key"},
		})
	})

	t.Run("member capture member, self", func(t *testing.T) {
		checkParameter(t, TestCase{
			code: `
			a = {
				"key": 1, 
				"get": () => a.key
			}
			target = a.get
			`,
			want: []string{
				"a", "#5.key",
			},
		})
	})
}

func TestClosure_Mask(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		checkMask(t, TestCase{
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
		checkMask(t, TestCase{
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

	// t.Run("object member", func(t *testing.T) {
	// 	checkMask(t, TestCase{
	// 		code: `
	// 		a = {
	// 			"key": 1,
	// 		}
	// 		f = () => {
	// 			a.key = 2
	// 		}
	// 		target = a.key
	// 		`,
	// 		want: []string{"2"},
	// 	})
	// })

	// t.Run("object member, not found", func(t *testing.T) {
	// 	checkMask(t, TestCase{
	// 		code: `
	// 	a = {}
	// 	f = () => {
	// 		a.key = 2
	// 	}
	// 	target = a.key
	// 	`,
	// 		want: []string{"2"},
	// 	})
	// })

	// t.Run("object member, self", func(t *testing.T) {
	// 	checkMask(t, TestCase{
	// 		code: `
	// 		a = {
	// 			"key": 1,
	// 			"set": (i) => {a.key = i}
	// 		}
	// 		target = a.key
	// 		`,
	// 		want: []string{},
	// 	})
	// })
}

func TestClosure_SideEffect(t *testing.T) {

	t.Run("function modify value", func(t *testing.T) {
		checkPrintlnValue(`
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
		checkPrintlnValue(`
		f  = () =>{}
		{
			a = 1
			f = () => {
				a = 2
			}
			println(a) // 1
			f()
			println(a) // 2
		}
		a = 1
		println(a) // 1 
		f() 
		println(a) // 1
		`, []string{
			"1", "side-effect(2, a)",
			"1", "1",
		}, t)
	})

	t.Run("object member modify value", func(t *testing.T) {
		checkPrintlnValue(`
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
		checkPrintlnValue(`
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
			"side-effect(Parameter-i, #6.key)",
		}, t)
	})

	t.Run("function modify object member, not found", func(t *testing.T) {
		checkPrintlnValue(`
		a =  {}
		f = (i) => {
			a.key = i
		}

		println(a.key) // undefined
		f(2) 
		println(a.key) // parameter-i
		`, []string{
			"Undefined-#2.key(valid)",
			"side-effect(Parameter-i, #5.key)",
		}, t)
	})

	t.Run("member modify member", func(t *testing.T) {
		checkPrintlnValue(`
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
			"side-effect(Parameter-i, #8.key)",
		}, t)
	})

	t.Run("object modify self", func(t *testing.T) {
		checkPrintlnValue(`
		a = {
			"key": 1,
			"add": (i) => {a.key = i},
		}
		println(a.key)
		a.add(2)
		println(a.key)
		`, []string{
			"1",
			"side-effect(Parameter-i, #7.key)",
		}, t)
	})
}
