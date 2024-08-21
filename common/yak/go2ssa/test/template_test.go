package test

import (
	"testing"

	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestTemplate_Type(t *testing.T) {
	t.Run("template type", func(t *testing.T) {
		test.CheckPrintlnValue(`package main
		type Queue[T int] struct {
			items []T
		}

		func (q *Queue[T]) Pop() T {
			item := q.items[0]
			q.items = q.items[1:]
			println(item)
		}

		`, []string{"Undefined-item(valid)"}, t)
	})
}

func TestTemplate_Function(t *testing.T) {
	t.Run("template function", func(t *testing.T) {
		test.CheckPrintlnValue(`package main

		func Pop[T int | string | bool](t T) T {
			return t
		}

		func main() {

			a := Pop[int](1)
			b := Pop[string]("1")
			c := Pop[bool](true)
			println(a)
			println(b)
			println(c)
		}
		`, []string{"Function-Pop(1)", "Function-Pop(\"1\")", "Function-Pop(true)"}, t)
	})
}
