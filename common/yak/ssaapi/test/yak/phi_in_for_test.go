package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPhiInCFG_If(t *testing.T) {
	code := `

input = cli.String("a")
if input.Contains("b") {
	input = input.Replace("b", "d")
}
os.System(input)

`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("os.System(* as $param,)").GetValues("param")
		if len(phis) == 0 {
			t.Fatalf("no phi found")
		}
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		if targetIns.CFGEntryBasicBlock > 0 {
			block, _ := targetIns.GetValueById(targetIns.CFGEntryBasicBlock)
			next, ok := block.IsCFGEnterBlock()
			if !ok {
				t.Fatal("not enter block")
			}
			_, ok = next[0].(*ssa.If)
			assert.True(t, ok)
		}
		return nil
	})
}

func TestPhiInCFG_If2(t *testing.T) {
	code := `

input = cli.String("a")
if input.Contains("b") {
	input = input.Replace("b", "d")
} else if e {
	input = input.Replace("b", "f")
}
os.System(input)

`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("os.System(* as $param,)").GetValues("param")
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		if targetIns.CFGEntryBasicBlock > 0 {
			block, _ := targetIns.GetValueById(targetIns.CFGEntryBasicBlock)
			next, ok := block.IsCFGEnterBlock()
			if !ok {
				t.Fatal("not enter block")
			}

			require.Equal(t, 2, len(next))
			_, ok = next[0].(*ssa.If)
			assert.True(t, ok)
			_, ok = next[1].(*ssa.If)
			assert.True(t, ok)

			assert.Equal(t, 2, len(next))
		}
		return nil
	})
}

func TestPhiInCFG_If_3(t *testing.T) {
	code := `

input = cli.String("a")
if input.Contains("b") {
	input = input.Replace("b", "d")
} else if e {
	input = input.Replace("b", "f")
} else {
	input = input.Replace("EEE", "FFF")
}
os.System(input)

`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("os.System(* as $param,)").GetValues("param")
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 2, len(conds))

		if targetIns.CFGEntryBasicBlock > 0 {
			block, _ := targetIns.GetValueById(targetIns.CFGEntryBasicBlock)
			next, ok := block.IsCFGEnterBlock()
			if !ok {
				t.Fatal("not enter block")
			}
			require.Equal(t, 2, len(next))
			_, ok = next[0].(*ssa.If)
			assert.True(t, ok)
			_, ok = next[1].(*ssa.If)
			assert.True(t, ok)
		}
		return nil
	})
}
func TestPhiInCFG_If_Without_ElseStatement(t *testing.T) {
	code := `
		a = 1
		if c{
			a=2
		}
		b = a
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("b as $b").GetValues("b")
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAInst())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		if targetIns.CFGEntryBasicBlock > 0 {
			block, _ := targetIns.GetValueById(targetIns.CFGEntryBasicBlock)
			next, ok := block.IsCFGEnterBlock()
			if !ok {
				t.Fatal("not enter block")
			}
			_, ok = next[0].(*ssa.If)
			assert.True(t, ok)
			// else statement should contain an if branch?
			// ignore else...
			assert.Equal(t, 1, len(next))
		}
		return nil
	})
}
func TestPhiMethod(t *testing.T) {
	code := `func a(){
    return "a"
}
func b(){
    return "b"
}
c = 1
var call
if(c){
    call = a
}else{
    call = b
}
result = call()
println(result)
`
	ssatest.Check(t, code, func(prog *ssaapi.Program) error {
		result, err := prog.SyntaxFlowWithError(`println(* #-> * as $param)`, ssaapi.QueryWithEnableDebug())
		require.NoError(t, err)
		param := result.GetValues("param")
		param.Show()
		require.True(t, param.Len() == 3)
		require.Contains(t, param.String(), "a")
		require.Contains(t, param.String(), "b")
		require.Contains(t, param.String(), "1")
		return nil
	}, ssaapi.WithLanguage(ssaapi.Yak))
}
func TestMethodCall(t *testing.T) {
	//	t.Run("test phi method call", func(t *testing.T) {
	//		code := `
	//func FuncA(b){
	//	//println(b)
	//}
	//func FuncC(b){
	//	//println(b)
	//}
	//var a = bada
	//if(c){
	// a =cada
	//}
	//println(a)
	//a(1)
	//`
	//		ssatest.CheckPrintlnValue(code, []string{}, t)
	//		//		ssatest.CheckSyntaxFlow(t, code, `FuncA() as $functionA
	//		//FuncC() as $functionC
	//		//`, map[string][]string{}, ssaapi.WithLanguage(ssaapi.Yak))
	//	})
	t.Run("sideEffect get callBy", func(t *testing.T) {
		code := `
func FuncA(a){
    println(a)
}
func FuncC(c){
    println(c)
}
var a = FuncA

func d(){
    a = FuncC
}
d()
a(1)`
		ssatest.CheckSyntaxFlow(t, code, `FuncC() as $functionC`, map[string][]string{
			"functionC": {"side-effect(FreeValue-FuncC, a)(1)"},
		}, ssaapi.WithLanguage(ssaapi.Yak))
	})
}
