package test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	test "github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestYaklangBasic_Variable_InBlock(t *testing.T) {
	t.Run("test simple assign", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a)
		a = 2
		println(a)
`, []string{
			"1",
			"2",
		}, t)
	})

	t.Run("simple test", func(t *testing.T) {
		test.CheckPrintlnValue(`
		println(a)
		`, []string{"Undefined-a"}, t)
	})

	t.Run("test sub-scope capture parent scope in basic block", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a)
		{
			a = 2
			println(a)
		}
		println(a)
		`, []string{
			"1",
			"2",
			"2",
		}, t)
	})

	t.Run("test sub-scope var local variable in basic block", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		{
			var a = 2
			println(a) // 2
		}
		println(a) // 1
		`, []string{
			"1",
			"2",
			"1",
		}, t)
	})

	t.Run("test sub-scope var local variable without assign in basic block", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		{
			var a
			println(a) // any
		}
		println(a) // 1
		`, []string{
			"1",
			"Undefined-a",
			"1",
		}, t)
	})

	t.Run("test sub-scope local variable in basic block", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		{
			a := 2
			println(a) // 2
		}
		println(a) // 1
		`, []string{
			"1",
			"2",
			"1",
		}, t)
	})

	t.Run("test sub-scope and return", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		{
			a  = 2 
			println(a) // 2
			return 
		}
		println(a) // unreachable
		`,
			[]string{
				"1", "2",
			}, t)
	})

	t.Run("undefine variable in sub-scope", func(t *testing.T) {
		test.CheckPrintlnValue(`
		{
			a = 2
			println(a) // 2
		}
		println(a) // undefine-a
		`, []string{
			"2",
			"Undefined-a",
		}, t)
	})

	t.Run("test ++ expression", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		{
			a ++
			println(a) // 2
		}
		`,
			[]string{
				"2",
			},
			t)
	})

	t.Run("test syntax block lose capture variable", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1 
		{
			a = 2  // capture [a: 2]
			{
				println(a) // 2
			} 
			// end-scope capture is []
		}
		println(a) // 2
		
		`, []string{
			"2", "2",
		}, t)
	})
}

func TestYaklangBasic_Variable_InIf(t *testing.T) {
	t.Run("test simple if", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a)
		if c {
			a = 2
			println(a)
		}
		println(a)
		`, []string{
			"1",
			"2",
			"phi(a)[2,1]",
		}, t)
	})
	t.Run("test simple if with local vairable", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a)
		if c {
			a := 2
			println(a)
		}
		println(a) // 1
		`, []string{
			"1",
			"2",
			"1",
		}, t)
	})

	t.Run("test multiple phi if", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		if c {
			a = 2
		}
		println(a)
		println(a)
		println(a)
		`, []string{
			"phi(a)[2,1]",
			"phi(a)[2,1]",
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("test multiple if ", func(t *testing.T) {
		test.CheckPrintlnValue(`
	a = 1
	if 1 {
		if 2 {
			a = 2
		}
	}
	println(a)
	`,
			[]string{
				"phi(a)[phi(a)[2,1],1]",
			},
			t)
	})

	t.Run("test simple if else", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a)
		if c {
			a = 2
			println(a)
		} else {
			a = 3
			println(a)
		}
		println(a)
		`, []string{
			"1",
			"2",
			"3",
			"phi(a)[2,3]",
		}, t)
	})

	t.Run("test simple if else with origin branch", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a)
		if c {
			// a = 1
		} else {
			a = 3
			println(a)
		}
		println(a) // phi(a)[1, 3]
		`, []string{
			"1",
			"3",
			"phi(a)[1,3]",
		}, t)
	})

	t.Run("test if-elseif", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a)
		if c {
			a = 2
			println(a)
		}else if  c == 2{
			a = 3
			println(a)
		}
		println(a)
		`,
			[]string{
				"1",
				"2",
				"3",
				"phi(a)[2,3,1]",
			}, t)
	})
	t.Run("test with return, no DoneBlock", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		if c {
			return 
		}
		println(a) // phi(a)[Undefined-a,1]
		`, []string{
			"1",
			"phi(a)[Undefined-a,1]",
		}, t)
	})
	t.Run("test with return in branch, no DoneBlock", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		println(a) // 1
		if c {
			if b {
				a = 2
				println(a) // 2
				return 
			}else {
				a = 3
				println(a) // 3
				return 
			}
			println(a) // unreachable // phi[2, 3]
		}
		println(a) // phi(a)[Undefined-a,1]
		`, []string{
			"1",
			"2",
			"3",
			"phi(a)[Undefined-a,1]",
		}, t)
	})

	t.Run("in if sub-scope", func(t *testing.T) {
		test.CheckPrintlnValue(`
		if c {
			a = 2
		}
		println(a)
		`, []string{"Undefined-a"}, t)
	})
}

func TestYaklangBasic_Variable_If_Logical(t *testing.T) {
	t.Run("test simple", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		if c || b {
			a = 2
		}
		println(a)
		`, []string{"phi(a)[2,1]"}, t)
	})

	t.Run("test multiple logical", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		if c || b && d {
			a = 2
		}
		println(a)
		`, []string{"phi(a)[2,1]"}, t)
	})
}

func TestYaklangBasic_variable_logical(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1 || 2 
		println(a)`,
			[]string{
				"phi(a)[1,2]",
			}, t)
	})

	t.Run("test ", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = () => {
			t = 1 || 2
			println(t)
		}
		a()
		`, []string{
			"phi(t)[1,2]",
		}, t)
	})
}

func TestYaklangBasic_Variable_Loop(t *testing.T) {
	t.Run("simple loop not change", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		for i=0; i < 10 ; i++ {
			println(a) // 1
		}
		println(a) //1 
		`,
			[]string{
				"1",
				"1",
			},
			t)
	})

	t.Run("simple loop only condition", func(t *testing.T) {
		test.CheckPrintlnValue(`
		i = 1
		for i < 10 { 
			println(i) // phi
			i = 2 
			println(i) // 2
		}
		println(i) // phi
		`, []string{
			"phi(i)[1,2]",
			"2",
			"phi(i)[1,2]",
		}, t)
	})

	t.Run("simple loop", func(t *testing.T) {
		test.CheckPrintlnValue(`
		i=0
		for i=0; i<10; i++ {
			println(i) // phi[0, i+1]
		}
		println(i)
		`,
			[]string{
				"phi(i)[0,add(i, 1)]",
				"phi(i)[0,add(i, 1)]",
			}, t)
	})

	t.Run("loop with spin, signal phi", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		for i := 0; i < 10; i ++ { // i=0; i=phi[0,1]; i=0+1=1
			println(a) // phi[0, $+1]
			a = 0
			println(a) // 0 
		}
		println(a)  // phi[0, 1]
		`,
			[]string{
				"phi(a)[1,0]",
				"0",
				"phi(a)[1,0]",
			},
			t)
	})

	t.Run("loop with spin, double phi", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		for i := 0; i < 10; i ++ {
			a += 1
			println(a) // add(phi, 1)
		}
		println(a)  // phi[1, add(phi, 1)]
		`,
			[]string{
				"add(phi(a)[1,add(a, 1)], 1)",
				"phi(a)[1,add(a, 1)]",
			},
			t)
	})
}

func TestYaklangParameter(t *testing.T) {
	check := func(code string, t *testing.T) {
		test := assert.New(t)
		prog, err := ssaapi.Parse(code)
		test.Nil(err)
		as := prog.Ref("a").ShowWithSource()
		test.Equal(1, len(as))
		test.Equal("a", as[0].GetRange().GetText())
	}
	t.Run("test parameter used", func(t *testing.T) {
		check(
			`
		f = (a) => {
			return a
		}
		`, t)
	})

	t.Run("test parameter not used", func(t *testing.T) {
		check(`
		f = (a) => {
			return 1
		}
		`, t)
	})

	t.Run("test free value used", func(t *testing.T) {
		check(`
		f = () => {
			return a
		}
		`, t)
	})
}

func TestYaklangBasic_Variable_Try(t *testing.T) {
	t.Run("simple, no final", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		try {
			a = 2
			println(a)
		} catch err {
			println(a)
			a = 3
		}
		println(a)`, []string{
			"2", "phi(a)[2,1]", "phi(a)[2,3]",
		}, t)
	})
	t.Run("simple, with final", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		try {
			a = 2
			println(a)
		} catch err {
			println(a) // phi(1, 2)
			a = 3
		} finally {
			println(a) // phi(2, 3)
		}
		println(a) // phi(2, 3)
		`, []string{
			"2", "phi(a)[2,1]", "phi(a)[2,3]", "phi(a)[2,3]",
		}, t)
	})

	t.Run("simple, no finally, has err", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		try {
		} catch err {
			println(err)
		}
		println(err)
		`, []string{
			"Undefined-err", "Undefined-err",
		}, t)
	})

	t.Run("simple, has finally, has err", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		try {
		} catch err {
			println(err)
		} finally {
			println(err)
		}
		println(err)
		`, []string{
			"Undefined-err",
			"Undefined-err",
			"Undefined-err",
		}, t)
	})
}

func TestYaklangBasic_Variable_Switch(t *testing.T) {
	t.Run("simple switch, no default", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		switch a {
		case 2: 
			a = 22
			println(a)
		case 3, 4:
			a = 33
			println(a)
		}
		println(a) // phi[1, 22, 33]
		`, []string{
			"22", "33", "phi(a)[22,33,1]",
		}, t)
	})

	t.Run("simple switch, has default but nothing", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		switch a {
		case 2: 
			a = 22
			println(a)
		case 3, 4:
			a = 33
			println(a)
		default: 
		}
		println(a) // phi[1, 22, 33]
		`, []string{
			"22", "33", "phi(a)[22,33,1]",
		}, t)
	})

	t.Run("simple switch, has default", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		switch a {
		case 2: 
			a = 22
			println(a)
		case 3, 4:
			a = 33
			println(a)
		default: 
			a = 44
			println(a)
		}
		println(a) // phi[22, 33, 44]
		`, []string{
			"22", "33", "44", "phi(a)[22,33,44]",
		}, t)
	})
}

func TestYaklangBasic_CFG_Break(t *testing.T) {
	t.Run("simple break in loop", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		for i := 0; i < 10; i++ {
			if i == 5 {
				a = 2
				break
			}
		}
		println(a) // phi[1, 2]
		`, []string{
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("simple continue in loop", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		for i := 0; i < 10; i++ {
			if i == 5 {
				a = 2
				continue
			}
		}
		println(a) // phi[1, 2]
		`, []string{
			"phi(a)[2,1]",
		}, t)
	})

	t.Run("simple break in switch", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		switch a {
		case 1:
			if c {
				a = 2
				break
			}
			a = 4
		case 2:
			a = 3
		}
		println(a) // phi[1, 2, 3, 4]
		`, []string{
			"phi(a)[2,4,3,1]",
		}, t)
	})

	t.Run("simple fallthrough in switch", func(t *testing.T) {
		test.CheckPrintlnValue(`
		a = 1
		switch a {
		case 1:
			a = 2
			fallthrough
		case 2:
			println(a) // 1 2
			a = 3
		default: 
			a = 4
		}
		println(a) // 3 4
		`, []string{
			"phi(a)[2,1]",
			"phi(a)[3,4]",
		}, t)
	})
}

func TestTemplateString(t *testing.T) {
	prog, err := ssaapi.Parse("a = 12; print(f`aaa${a}bbb`)", ssaapi.WithLanguage(ssaconfig.Yak))
	if err != nil {
		t.Fatal("prog parse error", err)
	}
	prog.Show()
}
