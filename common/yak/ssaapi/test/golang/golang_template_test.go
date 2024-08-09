package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Template(t *testing.T) {
	t.Run("type", func(t *testing.T) {
		code := `package main
		type Queue[T int] struct {
			items []T
		}

		func println(){}

		func (q *Queue[T]) Pop() T {
			item := q.items[0]
			q.items = q.items[1:]
			return item
		}

		func main(){
			q := &Queue[int]{items: []int{1,2,3}}
			a := q.Pop()
			println(a)
		}
		`
		ssatest.CheckSyntaxFlowEx(t, code, `
		println(* #-> as $a)
		`, true,map[string][]string{
			"a": {"1"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("function", func(t *testing.T) {
		code := `package main

		func Pop[T int | string | bool](t T) T {
			return t
		}

		func println[T int | string | bool](){}

		func main() {

			a := Pop[int](1)
			b := Pop[string]("1")
			c := Pop[bool](true)
			println(a)
			println(b)
			println(c)
		}
		`
		ssatest.CheckSyntaxFlowEx(t, code, `
		println(* #-> as $a)
		`, true,map[string][]string{
			"a": {"1", "\"1\"", "true"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}
