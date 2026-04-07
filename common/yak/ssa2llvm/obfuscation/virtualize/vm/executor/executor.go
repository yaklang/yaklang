// Package executor provides the virtualized function interpreter/executor.
//
// The executor runs PIR functions decoded from a protected blob. It uses a
// simple register-based virtual machine with a per-function register file.
// Host-calls bridge back to the native runtime via the HostCallHandler callback.
//
// This is the Phase 5 skeleton – it implements the core dispatch loop and
// arithmetic/comparison/control-flow opcodes. Full host-call integration with
// InvokeContext is deferred to runtime wiring.
package executor

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation/virtualize/vm/pir"
)

// HostCallHandler is called when the executor encounters an OpHostCall.
// callee is the register value holding the function reference.
// args contains the argument values. Returns the call result.
type HostCallHandler func(callee int64, args []int64) (int64, error)

// Frame holds execution state for one PIR function invocation.
type Frame struct {
	Func    *pir.Function
	Regs    []int64
	BlockPC int // current block index
	InstPC  int // current instruction index within block
}

// Result holds the return value of a PIR function execution.
type Result struct {
	Value int64
}

// Execute runs a PIR function with the given arguments.
// hostCall is invoked for OpHostCall instructions; nil means host-calls are unsupported.
func Execute(fn *pir.Function, args []int64, hostCall HostCallHandler) (*Result, error) {
	return executeFunction(nil, fn, args, hostCall)
}

// ExecuteRegion runs a named PIR function inside a decoded VM region.
func ExecuteRegion(region *pir.Region, entryName string, args []int64, hostCall HostCallHandler) (*Result, error) {
	if region == nil {
		return nil, fmt.Errorf("executor: nil region")
	}
	for _, fn := range region.Functions {
		if fn != nil && fn.Name == entryName {
			return executeFunction(region, fn, args, hostCall)
		}
	}
	return nil, fmt.Errorf("executor: virtualized function %q not found", entryName)
}

func executeFunction(region *pir.Region, fn *pir.Function, args []int64, hostCall HostCallHandler) (*Result, error) {
	if fn == nil {
		return nil, fmt.Errorf("executor: nil function")
	}
	if len(args) != fn.NumArgs {
		return nil, fmt.Errorf("executor: expected %d args, got %d", fn.NumArgs, len(args))
	}

	f := &Frame{
		Func:    fn,
		Regs:    make([]int64, fn.NumRegs),
		BlockPC: fn.EntryBlock,
	}

	// Load arguments into registers
	for i, v := range args {
		if i < fn.NumRegs {
			f.Regs[i] = v
		}
	}

	// Main dispatch loop
	for {
		if f.BlockPC < 0 || f.BlockPC >= len(fn.Blocks) {
			return nil, fmt.Errorf("executor: block index %d out of range", f.BlockPC)
		}
		block := &fn.Blocks[f.BlockPC]

		for f.InstPC = 0; f.InstPC < len(block.Insts); f.InstPC++ {
			inst := &block.Insts[f.InstPC]

			switch inst.Op {
			case pir.OpNop:
				// do nothing

			case pir.OpConst:
				f.Regs[inst.Dst] = inst.Imm

			case pir.OpArg:
				idx := int(inst.Imm)
				if idx < len(args) {
					f.Regs[inst.Dst] = args[idx]
				}

			// Arithmetic
			case pir.OpAdd:
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] + f.Regs[inst.Src[1]]
			case pir.OpSub:
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] - f.Regs[inst.Src[1]]
			case pir.OpMul:
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] * f.Regs[inst.Src[1]]
			case pir.OpDiv:
				rhs := f.Regs[inst.Src[1]]
				if rhs == 0 {
					return nil, fmt.Errorf("executor: division by zero")
				}
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] / rhs
			case pir.OpMod:
				rhs := f.Regs[inst.Src[1]]
				if rhs == 0 {
					return nil, fmt.Errorf("executor: modulo by zero")
				}
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] % rhs
			case pir.OpNeg:
				f.Regs[inst.Dst] = -f.Regs[inst.Src[0]]

			// Bitwise
			case pir.OpAnd:
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] & f.Regs[inst.Src[1]]
			case pir.OpOr:
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] | f.Regs[inst.Src[1]]
			case pir.OpXor:
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] ^ f.Regs[inst.Src[1]]
			case pir.OpShl:
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] << uint(f.Regs[inst.Src[1]])
			case pir.OpShr:
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] >> uint(f.Regs[inst.Src[1]])
			case pir.OpAndNot:
				f.Regs[inst.Dst] = f.Regs[inst.Src[0]] &^ f.Regs[inst.Src[1]]

			// Comparison
			case pir.OpEq:
				f.Regs[inst.Dst] = boolToInt(f.Regs[inst.Src[0]] == f.Regs[inst.Src[1]])
			case pir.OpNeq:
				f.Regs[inst.Dst] = boolToInt(f.Regs[inst.Src[0]] != f.Regs[inst.Src[1]])
			case pir.OpLt:
				f.Regs[inst.Dst] = boolToInt(f.Regs[inst.Src[0]] < f.Regs[inst.Src[1]])
			case pir.OpLe:
				f.Regs[inst.Dst] = boolToInt(f.Regs[inst.Src[0]] <= f.Regs[inst.Src[1]])
			case pir.OpGt:
				f.Regs[inst.Dst] = boolToInt(f.Regs[inst.Src[0]] > f.Regs[inst.Src[1]])
			case pir.OpGe:
				f.Regs[inst.Dst] = boolToInt(f.Regs[inst.Src[0]] >= f.Regs[inst.Src[1]])

			// Logical
			case pir.OpLogicAnd:
				f.Regs[inst.Dst] = boolToInt(f.Regs[inst.Src[0]] != 0 && f.Regs[inst.Src[1]] != 0)
			case pir.OpLogicOr:
				f.Regs[inst.Dst] = boolToInt(f.Regs[inst.Src[0]] != 0 || f.Regs[inst.Src[1]] != 0)

			// Control flow
			case pir.OpJump:
				f.BlockPC = inst.Block
				goto nextBlock

			case pir.OpBranch:
				if f.Regs[inst.Src[0]] != 0 {
					f.BlockPC = inst.Block
				} else {
					f.BlockPC = inst.AuxBlock
				}
				goto nextBlock

			case pir.OpReturn:
				var retVal int64
				if inst.Src[0] >= 0 {
					retVal = f.Regs[inst.Src[0]]
				}
				return &Result{Value: retVal}, nil

			case pir.OpPhi:
				// Phi in interpreter: select value from first edge.
				// A more complete implementation would track the previous block.
				if len(inst.Edges) > 0 {
					f.Regs[inst.Dst] = f.Regs[inst.Edges[0].Reg]
				}

			case pir.OpLoad:
				// placeholder for local slots
				f.Regs[inst.Dst] = 0
			case pir.OpStore:
				// placeholder

			case pir.OpCall:
				callee := f.Regs[inst.Src[0]]
				if region == nil {
					return nil, fmt.Errorf("executor: internal call requires region context")
				}
				index := int(callee)
				if index < 0 || index >= len(region.Functions) {
					return nil, fmt.Errorf("executor: internal callee index %d out of range", index)
				}
				callArgs := make([]int64, len(inst.CallArgs))
				for i, reg := range inst.CallArgs {
					callArgs[i] = f.Regs[reg]
				}
				result, err := executeFunction(region, region.Functions[index], callArgs, hostCall)
				if err != nil {
					return nil, fmt.Errorf("executor: internal call failed: %w", err)
				}
				if inst.Dst >= 0 {
					f.Regs[inst.Dst] = result.Value
				}

			case pir.OpHostCall:
				if hostCall == nil {
					return nil, fmt.Errorf("executor: host-call not configured")
				}
				callArgs := make([]int64, len(inst.CallArgs))
				for i, reg := range inst.CallArgs {
					callArgs[i] = f.Regs[reg]
				}
				callee := f.Regs[inst.Src[0]]
				result, err := hostCall(callee, callArgs)
				if err != nil {
					return nil, fmt.Errorf("executor: host-call failed: %w", err)
				}
				if inst.Dst >= 0 {
					f.Regs[inst.Dst] = result
				}

			default:
				return nil, fmt.Errorf("executor: unhandled opcode %s", inst.Op)
			}
		}

		// Fall through to next sequential block
		f.BlockPC++
		continue

	nextBlock:
		continue
	}
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
