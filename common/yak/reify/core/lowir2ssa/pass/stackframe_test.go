package pass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
)

const stackFrameLowIR = `target: linux/amd64 (64-bit, little-endian)
function f1 @ 0x9000
block b1 @ 0x9000
  0x9000  v1:i64 = read_register %rsp
  0x9000  v2:i64 = constant 0x8
  0x9000  v3:i64 = sub v1, v2
  0x9000  write_register %rsp, v3
  0x9001  v4:i64 = constant 0x10
  0x9001  write_register %rdi, v4
  0x9002  v5:i64 = read_register %rsp
  0x9002  v6:i64 = constant 0x8
  0x9002  v7:i64 = add v5, v6
  0x9002  write_register %rsp, v7
  0x9003  return
`

func stackAllocationFunction(t *testing.T) core.Function {
	t.Helper()
	ir, err := lowirtext.Parse(stackFrameLowIR)
	if err != nil {
		t.Fatal(err)
	}
	return ir.Functions[0]
}

func TestRemoveStackFrameDropsSetupAndTeardown(t *testing.T) {
	function := stackAllocationFunction(t)
	if removed := RemoveStackFrame(&function); removed == 0 {
		t.Fatal("stack frame pass removed nothing")
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instructions {
			if instruction.Kind != core.OpReadRegister && instruction.Kind != core.OpWriteRegister {
				continue
			}
			for _, operand := range instruction.Operands {
				if operand.Kind == core.OperandRegister && operand.Name == "rsp" {
					t.Fatalf("stack pointer instruction survived: %s", instruction.Kind)
				}
			}
		}
	}
}

func TestStackFrameWithFramePointer(t *testing.T) {
	ir := &core.LowIR{Target: testTarget(), Functions: []core.Function{stackAllocationFunction(t)}}
	ir.Functions[0].Blocks[0].Instructions = []core.Instruction{
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{register("rbp", 64)}, Address: 0x9100},
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 2, Type: integer(64)}}, Operands: []core.Operand{register("rsp", 64)}, Address: 0x9100},
		{Kind: core.OpConstant, Results: []core.Value{{ID: 3, Type: integer(64)}}, Operands: []core.Operand{literal(0x8)}, Address: 0x9100},
		{Kind: core.OpSub, Results: []core.Value{{ID: 4, Type: integer(64)}}, Operands: []core.Operand{value(2, integer(64)), value(3, integer(64))}, Address: 0x9100},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsp", 64), value(4, integer(64))}, Address: 0x9100},
		{Kind: core.OpCopy, Results: []core.Value{{ID: 5, Type: pointer()}}, Operands: []core.Operand{value(4, integer(64))}, Address: 0x9100},
		{Kind: core.OpStore, Operands: []core.Operand{value(5, pointer()), value(1, integer(64))}, Address: 0x9100},
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 6, Type: integer(64)}}, Operands: []core.Operand{register("rsp", 64)}, Address: 0x9101},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rbp", 64), value(6, integer(64))}, Address: 0x9101},
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 7, Type: integer(64)}}, Operands: []core.Operand{register("rsp", 64)}, Address: 0x9102},
		{Kind: core.OpConstant, Results: []core.Value{{ID: 8, Type: integer(64)}}, Operands: []core.Operand{literal(0x20)}, Address: 0x9102},
		{Kind: core.OpSub, Results: []core.Value{{ID: 9, Type: integer(64)}}, Operands: []core.Operand{value(7, integer(64)), value(8, integer(64))}, Address: 0x9102},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsp", 64), value(9, integer(64))}, Address: 0x9102},
		{Kind: core.OpConstant, Results: []core.Value{{ID: 10, Type: integer(32)}}, Operands: []core.Operand{literal(0)}, Address: 0x9103},
		{Kind: core.OpReturn, Operands: []core.Operand{value(10, integer(32))}, Address: 0x9104},
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 11, Type: integer(64)}}, Operands: []core.Operand{register("rbp", 64)}, Address: 0x9105},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsp", 64), value(11, integer(64))}, Address: 0x9105},
		{Kind: core.OpReadRegister, Results: []core.Value{{ID: 12, Type: integer(64)}}, Operands: []core.Operand{register("rsp", 64)}, Address: 0x9105},
		{Kind: core.OpCopy, Results: []core.Value{{ID: 13, Type: pointer()}}, Operands: []core.Operand{value(12, integer(64))}, Address: 0x9105},
		{Kind: core.OpLoad, Results: []core.Value{{ID: 14, Type: integer(64)}}, Operands: []core.Operand{value(13, pointer())}, Address: 0x9105},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rbp", 64), value(14, integer(64))}, Address: 0x9105},
		{Kind: core.OpConstant, Results: []core.Value{{ID: 15, Type: integer(64)}}, Operands: []core.Operand{literal(0x8)}, Address: 0x9105},
		{Kind: core.OpAdd, Results: []core.Value{{ID: 16, Type: integer(64)}}, Operands: []core.Operand{value(12, integer(64)), value(15, integer(64))}, Address: 0x9105},
		{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rsp", 64), value(16, integer(64))}, Address: 0x9105},
	}
	function := ir.Functions[0]
	if removed := RemoveStackFrame(&function); removed < 6 {
		t.Fatalf("removed %d frame instructions", removed)
	}
	for _, block := range ir.Functions[0].Blocks {
		for _, instruction := range block.Instructions {
			for _, operand := range instruction.Operands {
				if operand.Kind != core.OperandRegister {
					continue
				}
				if operand.Name == "rsp" || operand.Name == "rbp" {
					t.Fatalf("frame register %s remains in %s", operand.Name, instruction.Kind)
				}
			}
		}
	}
}
