package yak2ssa

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func check(t *testing.T, code string, regex string) {
	re, err := regexp.Compile(".*" + regex + ".*")
	if err != nil {
		t.Fatal(err)
	}
	prog := ParseSSA(code, func(fb *ssa.FunctionBuilder) {
		fb.WithExternMethod(&methodBuilder{})
	})
	prog.ShowWithSource()

	printlnFunc := prog.Packages[0].Funcs[0].GetValuesByName("println")[0]
	for _, final := range printlnFunc.GetUsers() {
		line := final.LineDisasm()
		fmt.Println(line)
		if !re.Match(utils.UnsafeStringToBytes(line)) {
			t.Fatal(line)
		}
	}
}

func TestCfgScope(t *testing.T) {
	code := `
a = 11
for a:=1; a<=10; a++{
	b = a
}
f= () => 1
switch f() {
    case 1: 
        a:=1
    default:
}
println("final:", a)
	`

	check(t, code, "11")
}

func TestPosition(t *testing.T) {
	code :=
		`
		b = 1
		for {
			a = b
		}
	`
	prog := ParseSSA(code, func(fb *ssa.FunctionBuilder) {})
	want := ssa.Position{
		SourceCode:  "1",
		StartLine:   2,
		StartColumn: 6,
		EndLine:     2,
		EndColumn:   7,
	}
	for _, v := range prog.InspectVariable("b").ProbablyValues {
		a := v.GetPosition()
		if *a != want {
			t.Error("phi get_position err")
		}
	}
}

func TestClosureSideEffect(t *testing.T) {
	code := `
	b = 1 
	f = () => {
		b = 2
	}
	// b = 1
	if c {
		f()
		// b = side-effect f()
	}
	println(b) // phi
	`

	check(t, code, "phi")
}

type methodBuilder struct {
}

func (b *methodBuilder) Build(t ssa.Type, name string) *ssa.FunctionType {
	strTyp := ssa.BasicTypes[ssa.String]
	switch t.GetTypeKind() {
	case ssa.String:
		switch name {
		case "join":
			f := ssa.NewFunctionTypeDefine("string.join", []ssa.Type{strTyp, strTyp}, []ssa.Type{strTyp}, false)
			f.SetModifySelf(true)
			return f
		}
	}
	return nil
}

func (b *methodBuilder) GetMethodNames(t ssa.Type) []string {
	switch t.GetTypeKind() {
	case ssa.String:
		return []string{"join"}
	}
	return nil
}

var _ ssa.MethodBuilder = (*methodBuilder)(nil)

func TestSelfModifyFunction(t *testing.T) {

	t.Run("basic line code", func(t *testing.T) {
		check(
			t,
			`
	a = "first line"
	a.join("second line")
	println(a)
		`,
			"second line",
		)
	})

	t.Run("basic line code2 multiple join", func(t *testing.T) {
		check(t,
			`
	a = "first line"
	a.join("second line")
	a.join("third line")
	println(a)
		`,
			`.*"second line".*"third line".*`)
	})

	t.Run("if cfg", func(t *testing.T) {
		check(t, `
	a = "first line" 
	if b == 1 {
		a.join("second line")
	}
	println(a)
	`, "second line")
	})

	t.Run("loop cfg", func(t *testing.T) {
		check(t, `
	a = "first line" 
	for item in list {
		a.join("second line")
	}
	println(a)
	`,
			`second line`,
		)
	})

	t.Run("loop cfg2 multiple join ", func(t *testing.T) {
		check(t, `
	a = "first line" 
	for item in list {
		a.join("second line")
		a.join("third line")
	}
	println(a)
	`, `"second line".*"third line"`)
	})

	t.Run("loop cfg with if", func(t *testing.T) {
		check(t, `
		a = "first line"
		for i in list {
			if i == "11" {
				a.join("second line")
			}
		}
		println(a)
		`, "second line")
	})

	t.Run("loop cfg with if2 multiple join ", func(t *testing.T) {
		check(t, `
		a = "first line"
		for i in list {
			if i == "11" {
				a.join("second line")
			}
			a.join("third line")
		}
		println(a)
		`, "third line")
	})
	t.Run("loop cfg with multiple if", func(t *testing.T) {
		check(t, `
		a = "first line"
		for i in list {
			if i == "11" {
				a.join("second line")
				// continue
			}
			if i == "22" {
				a.join("third line")
			}
		}
		println(a)
		`, `"second line".*"third line"`)
	})
}

func TestLineDisasm(t *testing.T) {
	code := `
	for i:=0; i<10; i++ {
		println(i)
	}
	`
	// i:
	// 	t0 = 0
	// 	t1 = phi[t0, t2]
	// 	t2 = t1 + 1
	// i : phi[0, (phi[0, (phi[0...] + 1)] + 1)]

	// i : phi[0, (...) + 1]
	// i : phi[0, (phi[0, (...)+1]) + 1]
	// i : $ = phi[0, $+1]
	// i : (lambda t0: (lambda t1: t1 + 1)(phi(t0, t0 + 1)))(0)
	// i : t0=>(t1=>(t1+1)(phi[t0, t0+1]))(0)
	// i : phi(i)[(init)0, (step)i+1]
	// i : phi(i)[0, i+1]

	check(t, code, "phi")
}

func TestCfg(t *testing.T) {

	t.Run("test multiple if", func(t *testing.T) {
		code := `
		if (a == 1) && (b == 1){
		}else if a == 2{
			if c == 1 {
			}else {
			}
		}elif a == 3 {
		}else {
		}
		`
		prog := ParseSSA(code, func(fb *ssa.FunctionBuilder) {
		})
		if prog == nil {
			t.Fatal("prog parse error")
		}
	})

	t.Run("test cfg Loop", func(t *testing.T) {
		code := `
		for (a && b) {
			if a == 1 {
			}
		}
		`
		prog := ParseSSA(code, func(fb *ssa.FunctionBuilder) {
		})
		if prog == nil {
			t.Fatal("prog parse error")
		}
		prog.Show()
	})

	t.Run("test if within try-catch", func(t *testing.T) {
		code := `
	scriptNameFile = "aa"
	try {
		if false{}
	} catch err {
		if false {  }
	}
	println(scriptNameFile)
	`
		check(t, code, "aa")
	})

	t.Run("test switch", func(t *testing.T) {
		code := `
		switch (a) {
			case 1 && 2:
				println(1)
			case 3:
				println(3)
			default:
				println(4)
		}
		`
		prog := ParseSSA(code, func(fb *ssa.FunctionBuilder) {
		})
		if prog == nil {
			t.Fatal("prog parse error")
		}
		prog.Show()

	})

	t.Run("test switch with if", func(t *testing.T) {
		code := `
		if (1) o = 0;
        else {
            switch (r) {
            case e:
            }
        }
        return
		`
		prog := ParseSSA(code, func (fb *ssa.FunctionBuilder)  {})
		if prog == nil {
			t.Fatal("prog parse error")
		}
		prog.Show()
	})

}

func TestSyntaxError(t *testing.T) {
	code := `
	a...91234yuerinsmzxkbc,vmkoqawiuflp][1[;yai]
	{ZXICv][ars]t[;]ar[setio][][][[][][]["""""""]]}]
	`
	prog := ParseSSA(code, func(fb *ssa.FunctionBuilder) {
	})
	if !utils.IsNil(prog) {
		t.Fatal("prog parse should error")
	}
}
