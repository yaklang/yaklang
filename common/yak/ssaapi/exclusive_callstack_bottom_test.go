package ssaapi

import (
	"testing"
)

func TestYaklang_BottomUses_Basic(t *testing.T) {
	t.Run("normal assign", func(t *testing.T) {
		Check(t,
			`var c = bbb
	var a = 55 + c
	myFunctionName(a)`,
			CheckBottomUser_Contain("c", []string{"myFunctionName("}),
		)
	})

	t.Run("const collapsed", func(t *testing.T) {
		Check(t,
			`var c = 1
	var a = 55 + c
	myFunctionName(a)`,
			CheckBottomUser_Contain("c", []string{"myFunctionName("}),
		)
	})
}

func TestYaklangExplore_BottomUses_BasicCallStack(t *testing.T) {
	Check(t, `
	var a = 1;
	b = i => i+1

	c = b(a)
	e = c+1

	sink = i => {
		println(i)
	}

	sink(e)
	`,
		CheckBottomUser_Contain("a", []string{"println("}),
	)
}

func TestYaklangExplore_BottomUses_CFG(t *testing.T) {
	Check(t, `
var c
var a = 1
if cond {
	a = c + 2
} else {
	a = c + 3
}

d = a;
myFunctionName(d)`,
		CheckBottomUser_Contain("c", []string{"myFunctionName("}),
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
		Check(t, code,
			CheckBottomUser_Contain("o", []string{"phi(a)["}, true),
		)
	})

}
