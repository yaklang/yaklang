package pass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
)

const canaryLowIR = `target: linux/amd64 (64-bit, little-endian)
function f1 @ 0x7000
block b1 @ 0x7000
  0x7000  v1:i64 = constant 0x28
  0x7000  v2:ptr64 = copy v1
  0x7000  v3:i64 = intrinsic @x86.segment.fs, v2 [read_register, unknown]
  0x7000  v4:i64 = load v3 [read_memory]
  0x7000  write_register %rsi, v4 [write_register]
  0x7009  v6:i64 = read_register %rbp [read_register]
  0x7009  v7:ptr64 = copy v6
  0x7009  v8:i64 = constant 0xfffffffffffffff8
  0x7009  v9:ptr64 = add v7, v8
  0x7009  v10:i64 = read_register %rsi [read_register]
  0x7009  store v9, v10 [write_memory]
  0x7010  v12:i64 = read_register %rbp [read_register]
  0x7010  v13:ptr64 = copy v12
  0x7010  v14:i64 = constant 0xfffffffffffffff8
  0x7010  v15:ptr64 = add v13, v14
  0x7010  v16:i64 = load v15 [read_memory]
  0x7010  write_register %rax, v16 [write_register]
  0x7014  v18:i64 = read_register %rax [read_register]
  0x7014  v19:i64 = constant 0x28
  0x7014  v20:ptr64 = copy v19
  0x7014  v21:i64 = intrinsic @x86.segment.fs, v20 [read_register, unknown]
  0x7014  v22:i64 = load v21 [read_memory]
  0x7014  v23:i64 = sub v18, v22
  0x7014  write_register %rax, v23 [write_register]
  0x7014  v25:i64 = intrinsic @x86.flags.sub, v18, v22, v23 [write_register]
  0x7014  write_register %rflags, v25 [write_register]
  0x701a  v27:i64 = read_register %rflags [read_register]
  0x701a  v28:i1 = intrinsic @x86.cond.e, v27 [read_register]
  0x701a  branch v28 -> b3, b2
block b2 @ 0x701c
  0x701c  v30:ptr64 = constant 0x9ef8
  0x701c  v31:i64 = load v30 [read_memory]
  0x701c  v32:ptr64 = copy v31
  0x701c  call v32 [call, may_trap, unknown]
  0x7022  jump -> b3
block b3 @ 0x7022
  0x7022  return [may_trap, unknown]
`

func canaryFunction(t *testing.T) core.Function {
	t.Helper()
	ir, err := lowirtext.Parse(canaryLowIR)
	if err != nil {
		t.Fatal(err)
	}
	return ir.Functions[0]
}

func TestRemoveStackCanaryDropsFailurePath(t *testing.T) {
	function := canaryFunction(t)
	if removed := RemoveStackCanary(&function); removed == 0 {
		t.Fatal("stack canary pass removed nothing")
	}
	if len(function.Blocks) != 2 {
		t.Fatalf("failure block survived: %d blocks", len(function.Blocks))
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instructions {
			for _, operand := range instruction.Operands {
				if operand.Kind != core.OperandSymbol {
					continue
				}
				switch operand.Name {
				case "x86.segment.fs", "x86.cond.e":
					t.Fatalf("canary instruction survived: %s", operand.Name)
				}
			}
			if instruction.Kind == core.OpCall {
				t.Fatal("guarded failure call survived")
			}
		}
	}
}

func TestRemoveStackCanaryLeavesUnrelatedFunctionsUntouched(t *testing.T) {
	function := functionWithBlocks(core.Block{
		ID:      1,
		Address: 0x1000,
		Instructions: []core.Instruction{
			{Kind: core.OpReturn, Address: 0x1000},
		},
	})
	before := len(function.Blocks)
	if removed := RemoveStackCanary(&function); removed != 0 {
		t.Fatalf("unrelated function changed: %d", removed)
	}
	if len(function.Blocks) != before {
		t.Fatal("unrelated function lost blocks")
	}
}

func TestRemoveStackCanaryRedirectsCheckBranch(t *testing.T) {
	function := canaryFunction(t)
	RemoveStackCanary(&function)
	for _, block := range function.Blocks {
		for _, instruction := range block.Instructions {
			if instruction.Kind != core.OpBranch {
				continue
			}
			if len(instruction.Targets) != 1 {
				t.Fatalf("check branch keeps %d targets", len(instruction.Targets))
			}
		}
	}
}
