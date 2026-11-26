package pass

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func returnWithRAX(address uint64) core.Instruction {
	return core.Instruction{
		Kind:     core.OpReturn,
		Operands: []core.Operand{register("rax", 64)},
		Address:  address,
	}
}

func TestFoldReturnTailsReplacesJumpToEpilogue(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{
			{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(64)}}, Operands: []core.Operand{literal(0x2a)}, Address: 0x1000},
			{Kind: core.OpWriteRegister, Operands: []core.Operand{register("rax", 64), value(1, integer(64))}, Address: 0x1000},
			jumpTo(2, 0x1000),
		}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{returnWithRAX(0x1004)}},
	)
	FoldReturnTails(&function, x86Facts())
	last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
	if last.Kind != core.OpReturn {
		t.Fatalf("tail jump survived: %s", last.Kind)
	}
	if len(last.Operands) != 1 || last.Operands[0].Name != "rax" {
		t.Fatalf("return does not use rax: %+v", last.Operands)
	}
}

func TestFoldReturnTailsKeepsLoopBackEdge(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1000)}},
		core.Block{ID: 2, Address: 0x1002, Instructions: []core.Instruction{{Kind: core.OpBranch, Targets: []core.BlockID{3, 1}, Address: 0x1002}}},
		core.Block{ID: 3, Address: 0x1004, Instructions: []core.Instruction{returnWithRAX(0x1004)}},
	)
	FoldReturnTails(&function, x86Facts())
	last := function.Blocks[1].Instructions[len(function.Blocks[1].Instructions)-1]
	if last.Kind != core.OpBranch {
		t.Fatalf("loop branch was folded away: %s", last.Kind)
	}
}

func TestFoldReturnTailsSkipsTailWithReturningCall(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1000)}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{
			{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "puts"}}, Address: 0x1004},
			returnWithRAX(0x1004),
		}},
	)
	FoldReturnTails(&function, x86Facts())
	last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
	if last.Kind != core.OpJump {
		t.Fatalf("tail with a returning call was folded: %s", last.Kind)
	}
}

func TestFoldReturnTailsFoldsAfterNonReturningCall(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1000)}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{
			{Kind: core.OpCall, Operands: []core.Operand{{Kind: core.OperandSymbol, Name: "exit"}}, Address: 0x1004},
			returnWithRAX(0x1004),
		}},
	)
	FoldReturnTails(&function, x86Facts())
	last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
	if last.Kind != core.OpReturn {
		t.Fatalf("tail after exit was not folded: %s", last.Kind)
	}
}

// x86Facts declares the return register of the architecture the existing cases
// were written for.
func x86Facts() core.ArchFacts {
	return core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingX86, Return: "rax", Width: 64}}
}

// TestFoldReturnTailsReadsTheDeclaredReturnRegister keeps the fold neutral: a
// PowerPC function returns in r3, so the folded return must read r3 and not a
// register that does not exist on the target.
func TestFoldReturnTailsReadsTheDeclaredReturnRegister(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{
			{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(32)}}, Operands: []core.Operand{literal(2)}, Address: 0x1000},
			{Kind: core.OpWriteRegister, Operands: []core.Operand{register("r3", 32), value(1, integer(32))}, Address: 0x1000},
			jumpTo(2, 0x1000),
		}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{returnWithRAX(0x1004)}},
	)
	FoldReturnTails(&function, core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingPlain, Return: "r3", Width: 32}})
	last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
	if last.Kind != core.OpReturn {
		t.Fatalf("tail jump survived: %s", last.Kind)
	}
	if len(last.Operands) != 1 || last.Operands[0].Name != "r3" || last.Operands[0].Type.Bits != 32 {
		t.Fatalf("return reads %v, want r3 at 32 bits", last.Operands)
	}
}

// TestFoldReturnTailsSkipsTailWritingNarrowReturnView keeps a tail that writes
// the 32-bit view of the declared 64-bit return register. A Clang -O0 shared
// epilogue loads the result slot into w0 and then returns; folding the jump to
// it would read the argument still in x0 and report that (issue 5496).
func TestFoldReturnTailsSkipsTailWritingNarrowReturnView(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1000)}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{
			{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(32)}}, Operands: []core.Operand{literal(7)}, Address: 0x1004},
			{Kind: core.OpWriteRegister, Operands: []core.Operand{register("w0", 32), value(1, integer(32))}, Address: 0x1004},
			returnWithRAX(0x1008),
		}},
	)
	FoldReturnTails(&function, core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingAArch64, Return: "x0", Width: 64}})
	last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
	if last.Kind != core.OpJump {
		t.Fatalf("tail writing w0 was folded: %s", last.Kind)
	}
}

// TestFoldReturnTailsFoldsAArch64DirectTail keeps the narrow-view change from
// blocking the fold on a function that really returns the argument untouched.
func TestFoldReturnTailsFoldsAArch64DirectTail(t *testing.T) {
	function := functionWithBlocks(
		core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1004)}},
		core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{returnWithRAX(0x1004)}},
	)
	FoldReturnTails(&function, core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingAArch64, Return: "x0", Width: 64}})
	last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
	if last.Kind != core.OpReturn {
		t.Fatalf("direct tail was not folded: %s", last.Kind)
	}
}

// crossArchReturn declares the facts of one architecture family so the table
// below drives the fold across every target the reify frontends cover.
type crossArchCase struct {
	name  string
	facts core.ArchFacts
}

func crossArchCases() []crossArchCase {
	return []crossArchCase{
		{"amd64", core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingX86, Return: "rax", Width: 64}}},
		{"x86", core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingX86, Return: "eax", Width: 32}}},
		{"aarch64", core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingAArch64, Return: "x0", Width: 64}}},
		{"arm", core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingPlain, Return: "r0", Width: 32}}},
		{"mips", core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingPlain, Return: "v0", Width: 32}}},
		{"ppc64", core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingPlain, Return: "r3", Width: 64}}},
		{"riscv64", core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingPlain, Return: "x10", Width: 64}}},
		{"s390x", core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingPlain, Return: "r2", Width: 64}}},
		{"loong64", core.ArchFacts{Registers: core.RegisterModel{Naming: core.RegisterNamingPlain, Return: "r4", Width: 64}}},
	}
}

// TestFoldReturnTailsCrossArchFoldsDirectTail checks that a pure return tail is
// folded on every architecture and the folded return reads the register the
// architecture declares.
func TestFoldReturnTailsCrossArchFoldsDirectTail(t *testing.T) {
	for _, tc := range crossArchCases() {
		function := functionWithBlocks(
			core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1004)}},
			core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{
				{Kind: core.OpReturn, Operands: []core.Operand{register(tc.facts.Registers.Return, tc.facts.Registers.Width)}, Address: 0x1004},
			}},
		)
		FoldReturnTails(&function, tc.facts)
		last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
		if last.Kind != core.OpReturn {
			t.Fatalf("%s: direct tail was not folded: %s", tc.name, last.Kind)
		}
		if len(last.Operands) != 1 || last.Operands[0].Name != tc.facts.Registers.Return {
			t.Fatalf("%s: folded return reads %v, want %s", tc.name, last.Operands, tc.facts.Registers.Return)
		}
	}
}

// TestFoldReturnTailsCrossArchSkipsTailWritingReturn keeps a shared epilogue
// that writes the return register on every architecture. The write makes the
// block a real epilogue, not a value-preserving tail, and folding it would
// report the argument instead of the result.
func TestFoldReturnTailsCrossArchSkipsTailWritingReturn(t *testing.T) {
	for _, tc := range crossArchCases() {
		function := functionWithBlocks(
			core.Block{ID: 1, Address: 0x1000, Instructions: []core.Instruction{jumpTo(2, 0x1004)}},
			core.Block{ID: 2, Address: 0x1004, Instructions: []core.Instruction{
				{Kind: core.OpConstant, Results: []core.Value{{ID: 1, Type: integer(tc.facts.Registers.Width)}}, Operands: []core.Operand{literal(7)}, Address: 0x1004},
				{Kind: core.OpWriteRegister, Operands: []core.Operand{register(tc.facts.Registers.Return, tc.facts.Registers.Width), value(1, integer(tc.facts.Registers.Width))}, Address: 0x1004},
				{Kind: core.OpReturn, Operands: []core.Operand{register(tc.facts.Registers.Return, tc.facts.Registers.Width)}, Address: 0x1008},
			}},
		)
		FoldReturnTails(&function, tc.facts)
		last := function.Blocks[0].Instructions[len(function.Blocks[0].Instructions)-1]
		if last.Kind != core.OpJump {
			t.Fatalf("%s: tail writing %s was folded: %s", tc.name, tc.facts.Registers.Return, last.Kind)
		}
	}
}
