package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func Test_Struct(t *testing.T) {
	t.Run("struct function inheritance", func(t *testing.T) {
		code := `package main

		type A struct {
			t int
		}

		type B struct {
			A
		}

		func (a *A) getA() int {
			return a.t
		}

		func (a *B) getA() int {
			return a.t
		}

		func main() {
			a := A{t: 1}
			b := B{A: A{t: 2}}

			a1 := a.getA()
			a2 := b.getA()
		}
		`

		ssatest.CheckSyntaxFlowEx(t, code, `
			a1 #-> as $a1
			a2 #-> as $a2
		`, true, map[string][]string{
			"a1": {"1"},
			"a2": {"2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})

	t.Run("struct function inheritance extend", func(t *testing.T) {
		code := `package main

		type A struct {
			t int
		}

		type B struct {
			t int
			A
		}

		func (a *A) getA() int {
			return a.t
		}

		func (a *B) getA() int {
			return a.t
		}
		
		func (a *B) getA2() int {
			return a.A.t
		}

		func main() {
			a := A{t: 1}
			b := B{A: A{t: 2}, t: 3}

			a1 := a.getA()
			a2 := b.getA()
			a3 := b.getA2()
		}
		`

		ssatest.CheckSyntaxFlowEx(t, code, `
		a1 #-> as $a1
		a2 #-> as $a2
		a3 #-> as $a3
		`, true, map[string][]string{
			"a1": {"1"},
			"a2": {"3"},
			"a3": {"2"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}

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
		`, true, map[string][]string{
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
		`, true, map[string][]string{
			"a": {"1", "\"1\"", "true"},
		}, ssaapi.WithLanguage(ssaapi.GO))
	})
}
