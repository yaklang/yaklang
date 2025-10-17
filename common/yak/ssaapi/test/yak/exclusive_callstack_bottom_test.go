package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestYaklang_BottomUses_Basic(t *testing.T) {
	t.Run("normal assign", func(t *testing.T) {
		ssatest.CheckBottomUser(t, `
	       var c = bbb
	       var a = 55 + c
	       myFunctionName(a)`,
			"c", []string{"myFunctionName("},
			true,
		)

		// 	ssatest.Check(t,
		// 		`var c = bbb
		// var a = 55 + c
		// myFunctionName(a)`,
		// 		ssatest.CheckBottomUser_Contain("c", []string{"myFunctionName("}),
		// 	)
	})

	t.Run("const collapsed", func(t *testing.T) {
		ssatest.CheckBottomUser(t,
			`var c = 1
       var a = 55 + c
       myFunctionName(a)`,
			"c", []string{"myFunctionName("},
			true,
		)
	})
}

func TestYaklangExplore_BottomUses_BasicCallStack(t *testing.T) {
	ssatest.CheckSyntaxFlowContain(t, `
	var a = 1;
	// b = i1 => i1+1

	// c = b(a)
	c = a 
	e = c+1

	sink = i => {
		println(i)
	}

	sink(e)
	`,
		`a --> as $target `,
		map[string][]string{
			"target": {
				"println(",
			},
		},
	)
}

func TestYaklangExplore_BottomUses_CFG(t *testing.T) {
	ssatest.CheckBottomUser(t, `
var c
var a = 1
if cond {
	a = c + 2
} else {
	a = c + 3
}

d = a;
myFunctionName(d)`,
		"c", []string{"myFunctionName("},
		true,
	)
}

func TestYaklang_SideEffect(t *testing.T) {
	t.Run("with cfg", func(t *testing.T) {
		code := `
		o = 5
		a = o
		b = () => {
			a = 2
		}
		if e {b()}
		c = a+1;
		`
		ssatest.CheckBottomUser(t, code,
			"o", []string{"phi(a)[", "FreeValue-a"},
			true,
		)
	})

}

func Test_Yaklang_BottomUser(t *testing.T) {
	code := `
		f = () =>{
			a = 11
			return a
		}
		f2 = (i) => {
			println(i)
		}
		t = f()
		f2(t)
		`
	t.Run("from return to other function", func(t *testing.T) {
		ssatest.CheckBottomUser(t, code,
			"a", []string{"println("},
			true,
		)
	})
	t.Run("from function", func(t *testing.T) {
		ssatest.CheckBottomUser(t, code,
			"f", []string{"println("},
			true,
		)
	})
}
