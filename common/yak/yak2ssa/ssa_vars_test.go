package yak2ssa

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

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
	prog := ParseSSA(code, func(fb *ssa.FunctionBuilder) {})
	// prog.Show()
	printlnFunc := prog.Packages[0].Funcs[0].GetValuesByName("println")[0]
	final := printlnFunc.GetUsers()[0]
	line := final.LineDisasm()
	if line != `println("final:",11)` {
		t.Error("final:", line)
	}
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
	prog := ParseSSA(code, func(fb *ssa.FunctionBuilder) {})
	printlnFunc := prog.Packages[0].Funcs[0].GetValuesByName("println")[0]
	final := printlnFunc.GetUsers()[0]
	line := final.LineDisasm()
	fmt.Println(line)
	if !strings.Contains(line, "phi") {
		t.Error("println: ", line)
	}
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
	code := `
	a = "first line"
	a.join("second line")
	println(a)
	a = "first line" 
	if b == 1 {
		a.join("second line")
	}
	println(a)
	`

	prog := ParseSSA(code, func(fb *ssa.FunctionBuilder) {
		fb.WithExternMethod(&methodBuilder{})
	})
	prog.ShowWithSource()

	printlnFunc := prog.Packages[0].Funcs[0].GetValuesByName("println")[0]
	for _, final := range printlnFunc.GetUsers() {
		line := final.LineDisasm()
		fmt.Println(line)
		if !strings.Contains(line, `"second line"`) {
			t.Error("println: ", line)
		}
	}
}
