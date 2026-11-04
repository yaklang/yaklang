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
	if len(parsers) != 10 {
		t.Fatalf("wanted 10 parsers, got %d", len(parsers))
	}
	seen := map[core.Architecture]bool{}
	for _, parser := range parsers {
		if seen[parser.Kind()] {
			t.Fatalf("duplicate architecture %s", parser.Kind())
		}
		seen[parser.Kind()] = true
		if parser.Available() != (parser.Kind() == core.ArchAMD64) {
			t.Fatalf("%s reported unexpected decoder availability", parser.Kind())
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
		{"amd64 big endian", target(core.ArchAMD64, 64, core.BigEndian, ""), false},
		{"amd64 32-bit", target(core.ArchAMD64, 32, core.LittleEndian, ""), false},
		{"386", target(core.Arch386, 32, core.LittleEndian, ""), true},
		{"386 big endian", target(core.Arch386, 32, core.BigEndian, ""), false},
		{"arm", target(core.ArchARM, 32, core.LittleEndian, ""), true},
		{"arm thumb", target(core.ArchARM, 32, core.BigEndian, "thumb"), true},
		{"arm mode", target(core.ArchARM, 32, core.LittleEndian, "aarch64"), false},
		{"arm 64-bit", target(core.ArchARM, 64, core.LittleEndian, ""), false},
		{"arm64", target(core.ArchARM64, 64, core.BigEndian, ""), true},
		{"arm64 mode", target(core.ArchARM64, 64, core.LittleEndian, "thumb"), false},
		{"riscv32", target(core.ArchRISCV32, 32, core.LittleEndian, ""), true},
		{"riscv64", target(core.ArchRISCV64, 64, core.LittleEndian, ""), true},
		{"mips", target(core.ArchMIPS, 32, core.BigEndian, ""), true},
		{"mips64", target(core.ArchMIPS64, 64, core.BigEndian, ""), true},
		{"ppc", target(core.ArchPPC, 32, core.BigEndian, ""), true},
		{"ppc64", target(core.ArchPPC64, 64, core.LittleEndian, ""), true},
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
	request := core.ParseRequest{Target: target(core.ArchAMD64, 64, core.LittleEndian, ""), Address: 0x1000, Code: []byte{0x90}}
	for _, parser := range All() {
		if parser.Kind() == core.ArchAMD64 {
			continue
		}
		request.Target.Architecture = parser.Kind()
		result, err := parser.Parse(context.Background(), request)
		if !errors.Is(err, ErrDecoderUnavailable) || result != nil {
			t.Fatalf("%s: result=%v err=%v", parser.Kind(), result, err)
		}
	}
}
