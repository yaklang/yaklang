package ssaapi

import (
	"testing"

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
	ssatest.CheckWithName("phi-in-for-case", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("os.System(* as $param,)").GetValues("param")
		if len(phis) == 0 {
			t.Fatalf("no phi found")
		}
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		if targetIns.CFGEntryBasicBlock > 0 {
			block := targetIns.GetValueById(targetIns.CFGEntryBasicBlock)
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
	ssatest.CheckWithName("phi-in-for-case", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("os.System(* as $param,)").GetValues("param")
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		if targetIns.CFGEntryBasicBlock > 0 {
			block := targetIns.GetValueById(targetIns.CFGEntryBasicBlock)
			next, ok := block.IsCFGEnterBlock()
			if !ok {
				t.Fatal("not enter block")
			}
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
	ssatest.CheckWithName("phi-in-for-case", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("os.System(* as $param,)").GetValues("param")
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 2, len(conds))

		if targetIns.CFGEntryBasicBlock > 0 {
			block := targetIns.GetValueById(targetIns.CFGEntryBasicBlock)
			next, ok := block.IsCFGEnterBlock()
			if !ok {
				t.Fatal("not enter block")
			}
			_, ok = next[0].(*ssa.If)
			assert.True(t, ok)
			_, ok = next[1].(*ssa.If)
			assert.True(t, ok)

			// else statement should contain an if branch?
			// ignore else...
			assert.Equal(t, 2, len(next))
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
	ssatest.CheckWithName("phi-in-for-case", t, code, func(prog *ssaapi.Program) error {
		prog.Show()
		phis := prog.SyntaxFlow("b as $b").GetValues("b")
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 1, len(conds))

		if targetIns.CFGEntryBasicBlock > 0 {
			block := targetIns.GetValueById(targetIns.CFGEntryBasicBlock)
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
