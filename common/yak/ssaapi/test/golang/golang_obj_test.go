package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBasic_BasicObject(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `package main

	type t struct {
		b int
		c int
	}

	func main(){
		a := t{}; 
		a.b = 1; 
		a.c = 3; 
		d := a.c + a.b
	}
	`,
			`d #-> as $target`,
			map[string][]string{
				"target": {"4"},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	t.Run("simple cross function", func(t *testing.T) {
		ssatest.CheckSyntaxFlowContain(t, `package main

	type t struct {
		b int
		c int
	}

	func f() t {
		return t{
			b: 1, 
			c: 3,
		}
	}
	func main(){
		a := f(); 
		d := a.c + a.b
	}
	`,
			`d #-> as $target`,
			map[string][]string{
				"target": {"3", "1"},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})
}

func TestBasic_BasicObjectEx(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t, `
		package main

		type Queue struct {
			mu    int
		}

		func NewQueue() *Queue {
			return &Queue{
				mu: 1,
			}
		}

		func main(){
			a := NewQueue()
			b := a.mu
		}
	`, `
		b #-> as $target
	`, map[string][]string{
		"target": {"1"},
	},
		ssaapi.WithLanguage(ssaapi.GO),
	)
}

func TestBasic_Phi(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t,
		`package main

	func main(){
		a := 0
		if (a > 0) {
			a = 1
		} else if (a > 1) {
			a = 2
		} else {
			a = 4
		}
		println(a)
	}
	`, `
	a ?{opcode: phi} as $p
	$p #-> as $target
	`, map[string][]string{
			"p":      {"phi(a)[1,2,4]"},
			"target": {"1", "2", "4"},
		},
		ssaapi.WithLanguage(ssaapi.GO),
	)
}

func TestBasic_BasicStruct(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t,
		`package main

type A struct {
	a int 
	b int 
	c int
}

func println(int) {}

func main (){
	t1 := &A{a:1,b:2,c:3}
	println(t1.a)
}
	`, `
	println(* #-> as $a)
	`, map[string][]string{
			"a": {"1"},
		},
		ssaapi.WithLanguage(ssaapi.GO),
	)
}
