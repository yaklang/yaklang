package executor_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/executor"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/pir"
)

// buildAddFunc constructs: func add(a, b) => a + b
func buildAddFunc() *pir.Function {
	return &pir.Function{
		Name:       "add",
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
}

func TestExecuteAdd(t *testing.T) {
	fn := buildAddFunc()
	result, err := executor.Execute(fn, []int64{10, 32}, nil)
	require.NoError(t, err)
	require.Equal(t, int64(42), result.Value)
}

func TestExecuteArithmetic(t *testing.T) {
	// func calc(a, b) => (a + b) * (a - b)
	fn := &pir.Function{
		Name: "calc", NumRegs: 5, NumArgs: 2, EntryBlock: 0,
		Blocks: []pir.Block{
			{
				Index: 0,
				Insts: []pir.Inst{
					{Op: pir.OpArg, Dst: 0, Imm: 0},
					{Op: pir.OpArg, Dst: 1, Imm: 1},
					{Op: pir.OpAdd, Dst: 2, Src: [2]int{0, 1}},
					{Op: pir.OpSub, Dst: 3, Src: [2]int{0, 1}},
					{Op: pir.OpMul, Dst: 4, Src: [2]int{2, 3}},
					{Op: pir.OpReturn, Dst: -1, Src: [2]int{4, 0}},
				},
			},
		},
	}
	// (10 + 5) * (10 - 5) = 15 * 5 = 75
	result, err := executor.Execute(fn, []int64{10, 5}, nil)
	require.NoError(t, err)
	require.Equal(t, int64(75), result.Value)
}

func TestExecuteConst(t *testing.T) {
	fn := &pir.Function{
		Name: "answer", NumRegs: 1, NumArgs: 0, EntryBlock: 0,
		Blocks: []pir.Block{
			{
				Index: 0,
				Insts: []pir.Inst{
					{Op: pir.OpConst, Dst: 0, Imm: 42},
					{Op: pir.OpReturn, Dst: -1, Src: [2]int{0, 0}},
				},
			},
		},
	}
	result, err := executor.Execute(fn, nil, nil)
	require.NoError(t, err)
	require.Equal(t, int64(42), result.Value)
}

func TestExecuteBranch(t *testing.T) {
	// func max(a, b) => if a > b then a else b
	fn := &pir.Function{
		Name: "max", NumRegs: 3, NumArgs: 2, EntryBlock: 0,
		Blocks: []pir.Block{
			{
				Index: 0,
				Insts: []pir.Inst{
					{Op: pir.OpArg, Dst: 0, Imm: 0},
					{Op: pir.OpArg, Dst: 1, Imm: 1},
					{Op: pir.OpGt, Dst: 2, Src: [2]int{0, 1}},
					{Op: pir.OpBranch, Dst: -1, Src: [2]int{2, 0}, Block: 1, AuxBlock: 2},
				},
			},
			{
				Index: 1,
				Insts: []pir.Inst{
					{Op: pir.OpReturn, Dst: -1, Src: [2]int{0, 0}},
				},
			},
			{
				Index: 2,
				Insts: []pir.Inst{
					{Op: pir.OpReturn, Dst: -1, Src: [2]int{1, 0}},
				},
			},
		},
	}

	// a > b: should return a
	result, err := executor.Execute(fn, []int64{10, 5}, nil)
	require.NoError(t, err)
	require.Equal(t, int64(10), result.Value)

	// a <= b: should return b
	result, err = executor.Execute(fn, []int64{3, 7}, nil)
	require.NoError(t, err)
	require.Equal(t, int64(7), result.Value)
}

func TestExecuteHostCall(t *testing.T) {
	// func caller() => hostcall(callee_id=0, args=[10, 20])
	fn := &pir.Function{
		Name: "caller", NumRegs: 4, NumArgs: 0, EntryBlock: 0,
		Blocks: []pir.Block{
			{
				Index: 0,
				Insts: []pir.Inst{
					{Op: pir.OpConst, Dst: 0, Imm: 0},  // callee "id"
					{Op: pir.OpConst, Dst: 1, Imm: 10}, // arg1
					{Op: pir.OpConst, Dst: 2, Imm: 20}, // arg2
					{Op: pir.OpHostCall, Dst: 3, Src: [2]int{0, 0}, CallArgs: []int{1, 2}},
					{Op: pir.OpReturn, Dst: -1, Src: [2]int{3, 0}},
				},
			},
		},
	}

	hostCall := func(callee int64, args []int64) (int64, error) {
		// Simple mock: return sum of args
		var sum int64
		for _, a := range args {
			sum += a
		}
		return sum, nil
	}

	result, err := executor.Execute(fn, nil, hostCall)
	require.NoError(t, err)
	require.Equal(t, int64(30), result.Value)
}

func TestExecuteJump(t *testing.T) {
	// func f() => jump to bb1, return 99
	fn := &pir.Function{
		Name: "f", NumRegs: 1, NumArgs: 0, EntryBlock: 0,
		Blocks: []pir.Block{
			{
				Index: 0,
				Insts: []pir.Inst{
					{Op: pir.OpJump, Dst: -1, Block: 1},
				},
			},
			{
				Index: 1,
				Insts: []pir.Inst{
					{Op: pir.OpConst, Dst: 0, Imm: 99},
					{Op: pir.OpReturn, Dst: -1, Src: [2]int{0, 0}},
				},
			},
		},
	}
	result, err := executor.Execute(fn, nil, nil)
	require.NoError(t, err)
	require.Equal(t, int64(99), result.Value)
}

func TestExecuteBitwise(t *testing.T) {
	fn := &pir.Function{
		Name: "bits", NumRegs: 4, NumArgs: 2, EntryBlock: 0,
		Blocks: []pir.Block{
			{
				Index: 0,
				Insts: []pir.Inst{
					{Op: pir.OpArg, Dst: 0, Imm: 0},
					{Op: pir.OpArg, Dst: 1, Imm: 1},
					{Op: pir.OpAnd, Dst: 2, Src: [2]int{0, 1}},
					{Op: pir.OpOr, Dst: 3, Src: [2]int{0, 1}},
					// result = (a & b) + (a | b) == a + b (MBA identity)
					{Op: pir.OpAdd, Dst: 2, Src: [2]int{2, 3}},
					{Op: pir.OpReturn, Dst: -1, Src: [2]int{2, 0}},
				},
			},
		},
	}
	result, err := executor.Execute(fn, []int64{0xFF, 0x0F}, nil)
	require.NoError(t, err)
	require.Equal(t, int64(0xFF+0x0F), result.Value)
}

func TestExecuteDivisionByZero(t *testing.T) {
	fn := &pir.Function{
		Name: "divzero", NumRegs: 3, NumArgs: 2, EntryBlock: 0,
		Blocks: []pir.Block{
			{
				Index: 0,
				Insts: []pir.Inst{
					{Op: pir.OpArg, Dst: 0, Imm: 0},
					{Op: pir.OpArg, Dst: 1, Imm: 1},
					{Op: pir.OpDiv, Dst: 2, Src: [2]int{0, 1}},
					{Op: pir.OpReturn, Dst: -1, Src: [2]int{2, 0}},
				},
			},
		},
	}
	_, err := executor.Execute(fn, []int64{10, 0}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "division by zero")
}

func TestExecuteNilFunction(t *testing.T) {
	_, err := executor.Execute(nil, nil, nil)
	require.Error(t, err)
}

func TestExecuteWrongArgCount(t *testing.T) {
	fn := buildAddFunc()
	_, err := executor.Execute(fn, []int64{1}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 2 args")
}

func TestHostCallBridge(t *testing.T) {
	bridge := executor.NewHostCallBridge()
	bridge.Register("add", func(args []int64) (int64, error) {
		return args[0] + args[1], nil
	})

	handler := bridge.Handler([]string{"add"})
	result, err := handler(0, []int64{3, 4})
	require.NoError(t, err)
	require.Equal(t, int64(7), result)
}
