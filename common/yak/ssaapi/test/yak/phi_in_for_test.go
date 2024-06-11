package ssaapi

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
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
		phis := prog.SyntaxFlowChain("os.System(* as $param,)").GetBySyntaxFlowName("param")
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		if targetIns.CFGEntryBasicBlock != nil {
			block := targetIns.CFGEntryBasicBlock.(*ssa.BasicBlock)
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
		phis := prog.SyntaxFlowChain("os.System(* as $param,)").GetBySyntaxFlowName("param")
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		if targetIns.CFGEntryBasicBlock != nil {
			block := targetIns.CFGEntryBasicBlock.(*ssa.BasicBlock)
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
		phis := prog.SyntaxFlowChain("os.System(* as $param,)").GetBySyntaxFlowName("param")
		phi := phis[0]
		phi.GetId()
		targetIns, ok := ssa.ToPhi(phi.GetSSAValue())
		if !ok {
			t.Fatal("not phi")
		}
		conds := targetIns.GetControlFlowConditions()
		assert.Equal(t, 2, len(conds))

		if targetIns.CFGEntryBasicBlock != nil {
			next, ok := targetIns.CFGEntryBasicBlock.IsCFGEnterBlock()
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
