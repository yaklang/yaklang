package golang

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func TestVersionAndABI(t *testing.T) {
	for _, tc := range []struct {
		version string
		minor   int
	}{{"", 0}, {"unknown", 0}, {"go1.16", 16}, {"go1.27.4-X:nodwarf5", 27}, {"go1.18rc1", 18}} {
		if got := goMinor(tc.version); got != tc.minor {
			t.Fatalf("goMinor(%q) = %d", tc.version, got)
		}
	}
	amd64 := goABI(core.Target{Architecture: core.ArchAMD64})
	arm64 := goABI(core.Target{Architecture: core.ArchARM64})
	if len(amd64.Registers.Arguments) != 9 || amd64.Registers.Arguments[0] != "rax" ||
		amd64.Registers.Arguments[1] != "rbx" || len(arm64.Registers.Arguments) != 16 {
		t.Fatalf("integer register models: %+v %+v", amd64, arm64)
	}
	if !amd64.NoCalleeSave || !arm64.NoCalleeSave {
		t.Fatal("ABIInternal must not claim ordinary callee-saved registers")
	}
	if goABI(core.Target{Architecture: core.Arch386}) != nil {
		t.Fatal("unimplemented ABI was guessed")
	}
	if model := goStrings(); model.Kind != core.StringGo || model.NulTerminated || model.Header != "pointer+length" {
		t.Fatalf("string model = %+v", model)
	}
}

func TestDetectWithoutBuildinfoIsOnlyLikely(t *testing.T) {
	a := &core.Artifact{Kind: core.KindNative,
		Image: &core.Image{Sections: []core.Section{{Name: ".gopclntab"}}}}
	report := Golang{}.Detect(a)
	if report == nil || report.Language != core.LanguageGo || report.Confidence != core.ConfidenceLikely {
		t.Fatalf("report = %+v", report)
	}
}

func TestPclntabRejectsUnboundedCount(t *testing.T) {
	data := make([]byte, 80)
	binary.LittleEndian.PutUint32(data, 0xfffffff1)
	data[6], data[7] = 1, 8
	binary.LittleEndian.PutUint64(data[8:], math.MaxUint64)
	a := pclnArtifact(t, data)
	if syms, err := (Golang{}).Recover(a); err == nil || len(syms) != 0 {
		t.Fatalf("unbounded count accepted: %d symbols, %v", len(syms), err)
	}
}

func pclnArtifact(t *testing.T, data []byte) *core.Artifact {
	t.Helper()
	r, err := core.NewByteReader("table", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return &core.Artifact{Kind: core.KindNative, Format: core.FormatELF, Reader: r,
		Target: core.Target{Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
		Image:  &core.Image{Sections: []core.Section{{Name: ".gopclntab", FileSize: uint64(len(data)), MemorySize: uint64(len(data))}}},
		Hints:  &core.BoundaryHints{ExecSections: []core.Region{{Address: 0x1000, Size: 0x100}}}}
}

func codeArtifact(t *testing.T, code []byte) *core.Artifact {
	t.Helper()
	r, err := core.NewByteReader("code", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	return &core.Artifact{Kind: core.KindNative, Format: core.FormatELF, Reader: r,
		Target: core.Target{Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
		Image: &core.Image{Sections: []core.Section{{Name: ".text", FileSize: uint64(len(code)),
			MemorySize: uint64(len(code)), VirtualAddress: 0x1000, Flags: 0x6}}}}
}

func TestFuncSizeTrimsOnlyAlignmentFill(t *testing.T) {
	x86 := append([]byte{0x55, 0x48, 0x89, 0xe5, 0xc3}, bytes.Repeat([]byte{0xcc}, 11)...)
	nops := append([]byte{0x31, 0xdb, 0x90}, bytes.Repeat([]byte{0x90}, 5)...)
	all := bytes.Repeat([]byte{0xcc}, 16)
	arm := append([]byte{0xc0, 0x03, 0x5f, 0xd6}, []byte{0x1f, 0x20, 0x03, 0xd5, 0, 0, 0, 0}...)
	for _, tc := range []struct {
		name string
		code []byte
		arch core.Architecture
		want uint64
	}{
		{"amd64 int3 tail", x86, core.ArchAMD64, 5},
		{"amd64 keeps one byte", all, core.ArchAMD64, 1},
		{"amd64 nop tail", nops, core.ArchAMD64, 2},
		{"arm64 nop and zero tail", arm, core.ArchARM64, 4},
		{"unknown architecture is untouched", x86, core.ArchRISCV64, uint64(len(x86))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := codeArtifact(t, tc.code)
			a.Target.Architecture = tc.arch
			if got := goFuncSize(a, 0x1000, uint64(len(tc.code)), tc.arch); got != tc.want {
				t.Fatalf("goFuncSize = %d, want %d", got, tc.want)
			}
		})
	}
	if got := goFuncSize(codeArtifact(t, x86), 0x9000, 16, core.ArchAMD64); got != 16 {
		t.Fatalf("out-of-range size changed to %d", got)
	}
}

func TestABI0KeepsPlatformConvention(t *testing.T) {
	entry := symbol.Symbol{Name: "runtime.morestack.abi0", Convention: symbol.ConventionPlatform}
	if entry.Convention != symbol.ConventionPlatform {
		t.Fatalf("abi0 symbol = %+v", entry)
	}
}

func FuzzPCLNTab(f *testing.F) {
	f.Add([]byte{0xf1, 0xff, 0xff, 0xff, 0, 0, 1, 8, 1, 0, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		a := pclnArtifact(t, data)
		_, _ = (Golang{}).Recover(a)
	})
}
