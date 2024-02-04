package test

import "testing"

func TestMemberCall(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		checkPrintlnValue(`
		a = {}
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	t.Run("normal slice", func(t *testing.T) {
		checkPrintlnValue(`
		a = [] 
		a[0] = 1
		println(a[0])
		`, []string{"1"}, t)
	})

}

func TestMemberCallNegative(t *testing.T) {

	/// check v
	t.Run("expr is undefine, create before", func(t *testing.T) {
		checkPrintlnValue(`
		b = a
		println(a.b)
		`, []string{"Undefined-a.b"}, t)
	})

	t.Run("expr is undefine, create right-now", func(t *testing.T) {
		checkPrintlnValue(`
		println(a.b)
		`, []string{"Undefined-a.b"}, t)
	})

	t.Run("expr conn't be index", func(t *testing.T) {
		checkPrintlnValue(`
		a = 1
		println(a.b)
		`, []string{"1.b"}, t)
	})

	// in left
	t.Run("expr is undefine in left", func(t *testing.T) {
		checkPrintlnValue(`
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})
	t.Run("expr is undefine, create before, in left", func(t *testing.T) {
		checkPrintlnValue(`
		b = a
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	t.Run("expr is, conn't be index, in left", func(t *testing.T) {
		checkPrintlnValue(`
		a = 1
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

	// expr = {}
	t.Run("expr is make", func(t *testing.T) {
		checkPrintlnValue(`
		a = {
			"A": 1,
		}

		println(a["A"])

		a["A"] = 2
		println(a["A"])
		`, []string{
			"1", "2",
		}, t)
	})

	// check key
	t.Run("expr normal, but undefine expr.key,", func(t *testing.T) {
		checkPrintlnValue(`
		v = {}
		println(v.key)
		`, []string{"make(map[any]any).key"}, t)
	})

	t.Run("expr normal, key is type", func(t *testing.T) {
		checkPrintlnValue(`
		v = "111"
		println(v[1])
		`, []string{
			`"111".1`,
		}, t,
		)
	})

}
