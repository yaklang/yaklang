package pir_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/pir"
)

func TestOpcodeString(t *testing.T) {
	require.Equal(t, "add", pir.OpAdd.String())
	require.Equal(t, "ret", pir.OpReturn.String())
	require.Equal(t, "hostcall", pir.OpHostCall.String())
	require.Equal(t, "nop", pir.OpNop.String())
}

func TestInstString(t *testing.T) {
	inst := pir.Inst{Op: pir.OpConst, Dst: 0, Imm: 42}
	require.Equal(t, "r0 = const 42", inst.String())

	inst2 := pir.Inst{Op: pir.OpAdd, Dst: 2, Src: [2]int{0, 1}}
	require.Equal(t, "r2 = add r0, r1", inst2.String())

	inst3 := pir.Inst{Op: pir.OpReturn, Dst: -1, Src: [2]int{2, 0}}
	require.Equal(t, "ret r2", inst3.String())

	inst4 := pir.Inst{Op: pir.OpJump, Dst: -1, Block: 1}
	require.Equal(t, "jump bb1", inst4.String())

	inst5 := pir.Inst{Op: pir.OpBranch, Dst: -1, Src: [2]int{3, 0}, Block: 1, AuxBlock: 2}
	require.Equal(t, "br r3, bb1, bb2", inst5.String())
}

func TestFunctionDump(t *testing.T) {
	fn := &pir.Function{
		Name:       "test_func",
		NumRegs:    3,
		NumArgs:    2,
		EntryBlock: 0,
		Blocks: []pir.Block{
			{
				Index: 0,
				Insts: []pir.Inst{
					{Op: pir.OpArg, Dst: 0, Imm: 0},
					{Op: pir.OpArg, Dst: 1, Imm: 1},
					{Op: pir.OpAdd, Dst: 2, Src: [2]int{0, 1}},
					{Op: pir.OpReturn, Dst: -1, Src: [2]int{2, 0}},
				},
			},
		},
	}

	dump := fn.Dump()
	require.Contains(t, dump, "pir func test_func")
	require.Contains(t, dump, "r2 = add r0, r1")
	require.Contains(t, dump, "ret r2")
}

func TestPhiInstString(t *testing.T) {
	inst := pir.Inst{
		Op:  pir.OpPhi,
		Dst: 5,
		Edges: []pir.PhiEdge{
			{Block: 0, Reg: 1},
			{Block: 1, Reg: 3},
		},
	}
	s := inst.String()
	require.True(t, strings.Contains(s, "phi"), "phi string: %s", s)
	require.True(t, strings.Contains(s, "bb0"), "phi string: %s", s)
	require.True(t, strings.Contains(s, "bb1"), "phi string: %s", s)
}

func TestHostCallInstString(t *testing.T) {
	inst := pir.Inst{
		Op:       pir.OpHostCall,
		Dst:      4,
		Src:      [2]int{0, 0},
		CallArgs: []int{1, 2, 3},
	}
	s := inst.String()
	require.Contains(t, s, "hostcall")
	require.Contains(t, s, "r1, r2, r3")
}

func TestRegionStruct(t *testing.T) {
	region := &pir.Region{
		Functions: []*pir.Function{
			{Name: "f1", NumRegs: 2, NumArgs: 1},
			{Name: "f2", NumRegs: 3, NumArgs: 2},
		},
		HostSymbols: []string{"print", "malloc"},
	}
	require.Len(t, region.Functions, 2)
	require.Len(t, region.HostSymbols, 2)
}
