package yak2ssa

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"regexp"
	"testing"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func ParseSSA(code string) (*ssa.Program, error) {
	return parseSSA(code, false, nil, func(fb *ssa.FunctionBuilder) {})
}
func check(t *testing.T, code string, regex string) {
	re, err := regexp.Compile(".*" + regex + ".*")
	if err != nil {
		t.Fatal(err)
	}
	prog, err := parseSSA(code, false, nil, func(fb *ssa.FunctionBuilder) {
		fb.WithExternMethod(&methodBuilder{})
	})
	if err != nil {
		t.Fatal(err)
	}

	prog.ShowWithSource()

	printlnFuncs := prog.GetAndCreateMainFunction().GetValuesByName("println")
	printlnFunc := printlnFuncs[0]
	for _, final := range printlnFunc.GetUsers() {
		line := ssa.LineDisasm(final)
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
	prog, err := ParseSSA(code)
	if err != nil {
		t.Fatal(err)
	}
	want := ssa.NewRange(ssa.NewPosition(7, 2, 6), ssa.NewPosition(7, 2, 7), "1")
	prog.ShowWithSource()
	vs := prog.GetAndCreateMainFunction().GetValuesByName("b")
	// for _, v := range vs {
	if len(vs) != 1 {
		t.Fatal("get Value b length error")
	}
	v := vs[0]
	a := v.GetRange()
	if *a.Start != *want.Start {
		t.Error("phi get_position start err: ", a)
	}
	if *a.End != *want.End {
		t.Error("phi get_position end err: ", a)
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
		prog, err := ParseSSA(code)
		if err != nil {
			t.Fatal("prog parse error")
		}
		_ = prog
	})

	t.Run("test cfg Loop", func(t *testing.T) {
		code := `
		for (a && b) {
			if a == 1 {
			}
		}
		`
		prog, err := ParseSSA(code)
		if err != nil {
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
		prog, err := ParseSSA(code)
		if err != nil {
			t.Fatal("prog parse error")
		}
		prog.Show()

	})

	t.Run("test switch with if", func(t *testing.T) {
		code := `
		if (1) {
			o = 0;
		} else {
            switch (r) {
            case e:
            }
        }
        return
		`
		prog, err := ParseSSA(code)
		if err != nil {
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
	code = `
	a.
	`
	prog, err := ParseSSA(code)
	if err == nil {
		t.Fatal("prog parse should error")
	}
	_ = prog
}

func TestVariable(t *testing.T) {
	t.Run("test variable basic: number and range", func(t *testing.T) {
		prog, err := ParseSSA(`
		a = 1
		{
			a := 2
		}
		a = 3
		`)
		if err != nil {
			t.Fatal("prog parse error", err)
		}
		vara := prog.GetAndCreateMainFunction().GetValuesByName("a")
		if len(vara) != 3 {
			t.Fatalf("error length: %s", vara)
		}
		// for _, v := range vara {

		// }
	})
	t.Run("basic function call", func(t *testing.T) {
		prog, err := ParseSSA(`println(a)`)
		if err != nil {
			t.Fatal("prog parse error", err)
		}
		prog.ShowWithSource()
		varA := prog.GetAndCreateMainFunction().GetValuesByName("a")
		if len(varA) != 1 {
			t.Fatal("value a length error: ", varA)
		}
		valueA := varA[0]
		if valueA.GetRange().Start.Offset != 8 {
			t.Fatal("value a offset error:", valueA.GetRange())
		}

	})
	t.Run("test variable left position", func(t *testing.T) {
		prog, err := ParseSSA(`
		a = 1 
		b = a 
		b = a + b
		println(b)
		c = a + 2
		`)
		if err != nil {
			t.Fatal("prog parse error", err)
		}
		main := prog.GetAndCreateMainFunction()
		{
			// check a
			varAs := main.GetValuesByName("a")
			if len(varAs) != 1 {
				t.Fatal("value a length error: ", varAs)
			}
			ValueA := varAs[0]
			variableA := ValueA.GetVariable("a")
			if variableA == nil {
				t.Fatal("variable a not exist !")
			}
			variableARange := variableA.Range
			if len(variableARange) != 4 {
				t.Fatal("variable range error", variableARange)
			}

			variableB := ValueA.GetVariable("b")
			if variableB == nil {
				t.Fatal("variable b not exist !")
			}
			variableBRange := variableB.Range
			if len(variableBRange) != 2 {
				t.Fatal("variable b range error", variableBRange)
			}
		}
		{
			varBs := main.GetValuesByName("b")
			if len(varBs) != 2 {
				t.Fatal("value b length error: ", varBs)
			}
			for _, value := range varBs {
				if value.String() == "1" {
					variableA := value.GetVariable("a")
					if variableA == nil {
						t.Fatal("variable a not exist! in valueB")
					}
					variableARange := variableA.Range
					if len(variableARange) != 4 {
						t.Fatal("variable a range error", variableARange)
					}

				}
			}
		}
	})
}

func TestExternLib(t *testing.T) {
	prog, err := parseSSA(`
	test.test()
	test.AAA()
	`, false, nil, func(fb *ssa.FunctionBuilder) {
		fb.ExternLib = map[string]map[string]any{
			"test": map[string]any{
				"test": func() {
					println("test")
				},
			},
		}
	})
	if err != nil {
		t.Fatal("prog parse error", err)
	}

	vs := prog.GetInstructionsByName("test")
	if len(vs) != 1 {
		t.Fatal("get test length error")
	}
	v, ok := ssa.ToUser(vs[0])
	if !ok {
		t.Fatal("get test value error")
	}
	values := v.GetValues()
	if len(values) != 2 {
		t.Fatal("get test value length error")
	}
	log.Info(values)
	want := []string{"test.test", "test.AAA"}
	// compare values and want
	for i := range values {
		if values[i].GetName() != want[i] {
			t.Fatal("get test value error want:", want[i], " vs got:", values[i].String())
		}
	}
}

func TestInclude(t *testing.T) {
	fileContent := `b=1; c = i => i;`
	fileName := consts.TempFileFast(fileContent)
	prog, err := ParseSSA(fmt.Sprintf(`include %#v; assert c(2) == 2; _ = e;`, fileName))
	if err != nil {
		t.Errorf("parse ssa failed: %v", err.Error())
		t.Failed()
	}
	if len(prog.GetErrors()) != 1 {
		t.Errorf("parse ssa failed: %v", prog.GetErrors())
		t.Failed()
	}
	var a = prog.GetFunctionFast("c")
	_ = a
	spew.Dump(prog.GetErrors())
}
