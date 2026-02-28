package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestClosureSideEffectMatrix(t *testing.T) {
	t.Run("basic captured variable write", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

func main() {
	a := 1
	f := func() {
		a = 2
	}
	f()
	println(a)
}
`, []string{"side-effect(2, a)"}, t)
	})

	t.Run("boundary closure defined but not invoked", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

func main() {
	a := 1
	f := func() {
		a = 2
	}
	_ = f
	println(a)
}
`, []string{
			"1",
		}, t)
	})

	t.Run("complex nested closure with branch and multi-call", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

func main() {
	a := 1
	outer := func() func(int) {
		return func(x int) {
			if x > 0 {
				a = 3
			} else {
				a = 4
			}
		}
	}
	update := outer()
	update(1)
	update(0)
	println(a)
}
`, []string{"side-effect(phi(a)[3,4], a)"}, t)
	})

	t.Run("boundary closure alias chain invoke", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

func main() {
	a := 1
	f := func() {
		a = 6
	}
	g := f
	h := g
	h()
	println(a)
}
`, []string{"side-effect(6, a)"}, t)
	})

	t.Run("complex variadic callback with branch and multi-call", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

func run(fn func(int), values ...int) {
	for _, v := range values {
		fn(v)
	}
}

func main() {
	a := 1
	update := func(v int) {
		if v > 0 {
			a = 7
		} else {
			a = 8
		}
	}
	run(update, 1, 0)
	println(a)
}
`, []string{"side-effect(phi(a)[7,8], a)"}, t)
	})
}
