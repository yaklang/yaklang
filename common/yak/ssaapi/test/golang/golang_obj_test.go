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
				"target": {"3", "1"},
			},
			ssaapi.WithLanguage(ssaapi.GO),
		)
	})

	// TODO: handler struct instance {}
	t.Run("simple cross function", func(t *testing.T) {
		t.Skip() // delete this

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
