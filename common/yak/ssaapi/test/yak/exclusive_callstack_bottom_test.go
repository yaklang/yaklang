package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestYaklang_BottomUses_Basic(t *testing.T) {
	t.Run("normal assign", func(t *testing.T) {
		ssatest.Check(t,
			`var c = bbb
	var a = 55 + c
	myFunctionName(a)`,
			ssatest.CheckBottomUser_Contain("c", []string{"myFunctionName("}),
		)
	})

	t.Run("const collapsed", func(t *testing.T) {
		ssatest.Check(t,
			`var c = 1
	var a = 55 + c
	myFunctionName(a)`,
			ssatest.CheckBottomUser_Contain("c", []string{"myFunctionName("}),
		)
	})
}

func TestYaklangExplore_BottomUses_BasicCallStack(t *testing.T) {
	ssatest.Check(t, `
	var a = 1;
	b = i => i+1

	c = b(a)
	e = c+1

	sink = i => {
		println(i)
	}

	sink(e)
	`,
		ssatest.CheckBottomUser_Contain("a", []string{"println("}),
	)
}

func TestYaklangExplore_BottomUses_CFG(t *testing.T) {
	ssatest.Check(t, `
var c
var a = 1
if cond {
	a = c + 2
} else {
	a = c + 3
}

d = a;
myFunctionName(d)`,
		ssatest.CheckBottomUser_Contain("c", []string{"myFunctionName("}),
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
		ssatest.Check(t, code,
			ssatest.CheckBottomUser_Contain("o", []string{"phi(a)[", "FreeValue-a"}, true),
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
		ssatest.Check(t, code,
			ssatest.CheckBottomUser_Contain("a", []string{"println("}),
		)
	})
	t.Run("from function", func(t *testing.T) {
		ssatest.Check(t, code,
			ssatest.CheckBottomUser_Contain("f", []string{"println("}),
		)
	})
}

// func TestA(t *testing.T) {
// 	opt := static_analyzer.GetPluginSSAOpt("yak")
// 	opt = append(opt, ssaapi.WithLanguage(ssaapi.Yak))
// 	prog, err := ssaapi.ParseFromString(`
// 	ssa.Parse("",ssa.withLanguage(ssa.Yak))
// 	ssa.Parse("")
// 	`,
// 		opt...,
// 	)
// 	require.NoError(t, err)

// 	// prog.Program.ShowWithSource()
// 	prog.Show()
// 	prog.Ref("ssa").GetOperands().ShowWithSource()
// }
