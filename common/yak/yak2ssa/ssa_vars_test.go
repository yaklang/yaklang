package yak2ssa

import (
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
