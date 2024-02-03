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
	t.Run("undefine expr", func(t *testing.T) {
		checkPrintlnValue(`
		a.b = 1
		println(a.b)
		`, []string{"1"}, t)
	})

}
