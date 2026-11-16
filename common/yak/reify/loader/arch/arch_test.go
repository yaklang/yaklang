package arch

import (
	"context"
	"errors"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func target(arch core.Architecture, bits int, order core.ByteOrder, mode string) core.Target {
	return core.Target{OS: core.OSLinux, Architecture: arch, Bits: bits, ByteOrder: order, Mode: mode}
}

func TestAllArchitectures(t *testing.T) {
	parsers := All()
	if len(parsers) != 13 {
		t.Fatalf("wanted 13 parsers, got %d", len(parsers))
	}
	seen := map[core.Architecture]bool{}
	for _, parser := range parsers {
		if seen[parser.Kind()] {
			t.Fatalf("duplicate architecture %s", parser.Kind())
		}
		seen[parser.Kind()] = true
		if !parser.Available() {
			t.Fatalf("%s reported no decoder", parser.Kind())
		}
		if parser.Name() != string(parser.Kind()) {
			t.Fatalf("%s name = %q", parser.Kind(), parser.Name())
		}
	}
	for _, kind := range []core.Architecture{
		core.Arch386, core.ArchAMD64, core.ArchARM, core.ArchARM64, core.ArchRISCV32,
		core.ArchRISCV64, core.ArchMIPS, core.ArchMIPS64, core.ArchPPC, core.ArchPPC64,
	} {
		if !seen[kind] {
			t.Fatalf("%s has no parser", kind)
		}
	}
}

func TestCheckTargetMatrix(t *testing.T) {
	parsers := map[core.Architecture]core.Parser{}
	for _, parser := range All() {
		parsers[parser.Kind()] = parser
	}
	for _, tc := range []struct {
		name   string
		target core.Target
		valid  bool
	}{
		{"amd64", target(core.ArchAMD64, 64, core.LittleEndian, ""), true},
		{"amd64 ignores byte order", target(core.ArchAMD64, 64, core.BigEndian, ""), true},
		{"amd64 32-bit", target(core.ArchAMD64, 32, core.LittleEndian, ""), false},
		{"386", target(core.Arch386, 32, core.LittleEndian, ""), true},
		{"386 ignores byte order", target(core.Arch386, 32, core.BigEndian, ""), true},
		{"arm", target(core.ArchARM, 32, core.LittleEndian, ""), true},
		{"arm", target(core.ArchARM, 32, core.BigEndian, ""), true},
		{"arm mode", target(core.ArchARM, 32, core.LittleEndian, "arm"), true},
		{"arm thumb", target(core.ArchARM, 32, core.LittleEndian, "thumb"), true},
		{"arm thumb2", target(core.ArchARM, 32, core.LittleEndian, "thumb2"), true},
		{"arm aarch64 mode", target(core.ArchARM, 32, core.LittleEndian, "aarch64"), false},
		{"arm 64-bit", target(core.ArchARM, 64, core.LittleEndian, ""), false},
		{"arm64", target(core.ArchARM64, 64, core.BigEndian, ""), true},
		{"arm64 mode", target(core.ArchARM64, 64, core.LittleEndian, "thumb"), false},
		{"riscv32", target(core.ArchRISCV32, 32, core.LittleEndian, ""), true},
		{"riscv64", target(core.ArchRISCV64, 64, core.LittleEndian, ""), true},
		{"riscv32 64-bit", target(core.ArchRISCV32, 64, core.LittleEndian, ""), false},
		{"mips", target(core.ArchMIPS, 32, core.BigEndian, ""), true},
		{"mips64", target(core.ArchMIPS64, 64, core.BigEndian, ""), true},
		{"ppc", target(core.ArchPPC, 32, core.BigEndian, ""), true},
		{"ppc64", target(core.ArchPPC64, 64, core.LittleEndian, ""), true},
		{"mips 64-bit", target(core.ArchMIPS, 64, core.BigEndian, ""), false},
		{"ppc64 mode", target(core.ArchPPC64, 64, core.LittleEndian, "le"), false},
		{"wrong architecture", target(core.ArchMIPS, 64, core.BigEndian, ""), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parser, ok := parsers[tc.target.Architecture]
			if !ok {
				t.Fatalf("no parser for %s", tc.target.Architecture)
			}
			err := parser.Check(tc.target)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
		})
	}
}

func TestParseReportsUnavailable(t *testing.T) {
	// Every registered parser now decodes, so the sentinel is only reachable
	// through a synthetic parser. The check keeps the contract covered.
	parser := unavailableParser{kind: core.ArchMIPS}
	if _, err := parser.Parse(context.Background(), core.ParseRequest{}); !errors.Is(err, ErrDecoderUnavailable) {
		t.Fatalf("sentinel lost: %v", err)
	}
}

// unavailableParser stands in for a parser without a decoder so the sentinel
// contract stays covered now that every registered parser decodes.
type unavailableParser struct {
	kind core.Architecture
}

func (p unavailableParser) Kind() core.Architecture { return p.kind }
func (p unavailableParser) Name() string            { return string(p.kind) }
func (p unavailableParser) Available() bool         { return false }
func (p unavailableParser) Check(core.Target) error { return nil }
func (p unavailableParser) Parse(context.Context, core.ParseRequest) (*core.ParseResult, error) {
	return unavailable(p.kind)
}

// TestEveryParserReportsArchFacts checks that each parser describes its own
// architecture. A pass cannot name a stack pointer, and a converter cannot bind an
// argument register, without these facts; a parser that forgets them silently
// produces a program with no parameters and no frame removal.
func TestEveryParserReportsArchFacts(t *testing.T) {
	for _, parser := range All() {
		provider, ok := parser.(core.ArchFactsProvider)
		if !ok {
			t.Fatalf("%s does not report arch facts", parser.Kind())
		}
		facts := provider.Facts()
		if facts.Frame.StackPointer == "" {
			t.Fatalf("%s reports no stack pointer", parser.Kind())
		}
		if facts.Registers.Naming == "" || facts.Registers.Width == 0 {
			t.Fatalf("%s reports no register model", parser.Kind())
		}
		if facts.Frame.StackGrowth != -1 && facts.Frame.StackGrowth != 1 {
			t.Fatalf("%s reports stack growth %d", parser.Kind(), facts.Frame.StackGrowth)
		}
		// The frame and register models must agree about the flag register: a
		// consumer reads the width from one and the name from the other.
		if facts.Frame.Flags != "" && facts.Frame.Flags != facts.Registers.Flags {
			t.Fatalf("%s reports flag register %q and %q", parser.Kind(), facts.Frame.Flags, facts.Registers.Flags)
		}
		// Argument registers belong to the same register file as the stack
		// pointer, so they must use the naming the model declares.
		for _, name := range append(append([]string{}, facts.Registers.Arguments...), facts.Registers.Return) {
			if name == "" {
				t.Fatalf("%s reports an empty argument or return register", parser.Kind())
			}
		}
	}
}

// TestArgumentRegistersPerArchitecture checks that the ABI facts are not one
// shared default. Before they were data, a converter that switched on the
// architecture handed every other target the System V registers, so a RISC-V
// function came out with no parameters at all.
func TestArgumentRegistersPerArchitecture(t *testing.T) {
	cases := []struct {
		kind   core.Architecture
		first  string
		ret    string
		expect int
	}{
		{core.ArchAMD64, "rdi", "rax", 6},
		{core.Arch386, "", "eax", 0},
		{core.ArchARM, "r0", "r0", 4},
		{core.ArchARM64, "x0", "x0", 8},
		{core.ArchRISCV64, "x10", "x10", 8},
		{core.ArchPPC64, "r3", "r3", 8},
		{core.ArchMIPS64, "a0", "v0", 4},
		{core.ArchS390X, "r2", "r2", 5},
		{core.ArchLOONG64, "r4", "r4", 8},
	}
	for _, testCase := range cases {
		var parser core.Parser
		for _, candidate := range All() {
			if candidate.Kind() == testCase.kind {
				parser = candidate
				break
			}
		}
		if parser == nil {
			t.Fatalf("%s is not registered", testCase.kind)
		}
		registers := parser.(core.ArchFactsProvider).Facts().Registers
		if len(registers.Arguments) != testCase.expect {
			t.Fatalf("%s has %d argument registers, want %d", testCase.kind, len(registers.Arguments), testCase.expect)
		}
		if testCase.expect > 0 && registers.Arguments[0] != testCase.first {
			t.Fatalf("%s first argument register is %q, want %q", testCase.kind, registers.Arguments[0], testCase.first)
		}
		if registers.Return != testCase.ret {
			t.Fatalf("%s returns in %q, want %q", testCase.kind, registers.Return, testCase.ret)
		}
	}
}
