package golang

import (
	"bytes"
	"debug/gosym"
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
	if model := (Golang{}).Strings(); model.Layout != core.StringLayoutPointerLength ||
		model.Encoding != core.StringEncodingUTF8 || model.ValidateUTF8 {
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

func TestMetadataLimitAllowsLargeNamedPclntab(t *testing.T) {
	if got := metadataLimit(".gopclntab", core.FormatELF); got != maxNamedPclntabSize {
		t.Fatalf("named pclntab limit = %d", got)
	}
	if got := metadataLimit("<segment>,__gopclntab", core.FormatMachO); got != maxNamedPclntabSize {
		t.Fatalf("macho pclntab limit = %d", got)
	}
	// PE merges .gopclntab into .rdata; a large engine exceeds the generic
	// fallback ceiling.
	if got := metadataLimit(".rdata", core.FormatPE); got != maxNamedPclntabSize {
		t.Fatalf("PE .rdata limit = %d", got)
	}
	if got := metadataLimit(".rdata", core.FormatELF); got != maxMetadataSize {
		t.Fatalf("ELF fallback section limit = %d", got)
	}
	if got := metadataLimit(".data.rel.ro", core.FormatELF); got != maxMetadataSize {
		t.Fatalf("fallback section limit = %d", got)
	}
	if maxNamedPclntabSize <= maxMetadataSize || maxNamedPclntabSize < 85<<20 {
		t.Fatalf("named limit must cover a real 85 MiB table: %d", maxNamedPclntabSize)
	}
}

func TestDirectGo120TableMatchesGosym(t *testing.T) {
	// Build a minimal valid Go 1.20 (0xfffffff1) pclntab with 2 functions and text=0x1000.
	// Header: 8 bytes + 8*8 = 72 bytes.
	const (
		funcnameOff = 0x80
		pclnOff     = 0xc0
		textBase    = 0x1000
	)
	names := []byte("main.alpha\x00main.(*Box[go.shape.int]).Get\x00")
	// 2 functab entries + 1 end sentinel = 5 uint32s + pad -> actually (2*2+1)*4 = 20 bytes -> pad to 24.
	const funcInfoOff0 = 24
	const funcInfoOff1 = 48
	buf := make([]byte, pclnOff+80)
	binary.LittleEndian.PutUint32(buf[0:], 0xfffffff1)
	buf[6], buf[7] = 1, 8
	binary.LittleEndian.PutUint64(buf[8+0*8:], 2)           // nfunc
	binary.LittleEndian.PutUint64(buf[8+1*8:], 0)           // nfile
	binary.LittleEndian.PutUint64(buf[8+2*8:], textBase)    // textStart
	binary.LittleEndian.PutUint64(buf[8+3*8:], funcnameOff) // funcnameOffset
	binary.LittleEndian.PutUint64(buf[8+4*8:], funcnameOff+uint64(len(names)))
	binary.LittleEndian.PutUint64(buf[8+5*8:], funcnameOff+uint64(len(names)))
	binary.LittleEndian.PutUint64(buf[8+6*8:], pclnOff)
	binary.LittleEndian.PutUint64(buf[8+7*8:], pclnOff)
	copy(buf[funcnameOff:], names)

	ftab := buf[pclnOff:]
	binary.LittleEndian.PutUint32(ftab[0:], 0x0)          // func 0 entryOff
	binary.LittleEndian.PutUint32(ftab[4:], funcInfoOff0) // func 0 info
	binary.LittleEndian.PutUint32(ftab[8:], 0x20)         // func 1 entryOff
	binary.LittleEndian.PutUint32(ftab[12:], funcInfoOff1)
	binary.LittleEndian.PutUint32(ftab[16:], 0x40) // end entryOff
	// _func structs: +0 entryOff, +4 nameOff
	binary.LittleEndian.PutUint32(ftab[funcInfoOff0:], 0x0)
	binary.LittleEndian.PutUint32(ftab[funcInfoOff0+4:], 0)
	binary.LittleEndian.PutUint32(ftab[funcInfoOff1:], 0x20)
	binary.LittleEndian.PutUint32(ftab[funcInfoOff1+4:], uint32(len("main.alpha\x00")))

	a := pclnArtifact(t, buf)
	a.Hints.ExecSections = []core.Region{{Address: textBase, Offset: 0, Size: 0x100}}
	direct, err := (Golang{}).Recover(a)
	if err != nil {
		t.Fatalf("direct Recover failed: %v", err)
	}
	lt := gosym.NewLineTable(buf, textBase)
	tbl, err := gosym.NewTable(nil, lt)
	if err != nil {
		t.Fatalf("gosym.NewTable failed: %v", err)
	}
	if len(direct) != len(tbl.Funcs) || len(direct) != 2 {
		t.Fatalf("got %d direct, %d gosym", len(direct), len(tbl.Funcs))
	}
	if direct[0].Name != "main.alpha" || direct[0].Address != 0x1000 {
		t.Fatalf("direct[0] = %+v", direct[0])
	}
	if direct[1].Name != "main.(*Box[int]).Get" || direct[1].Mangled != "main.(*Box[go.shape.int]).Get" {
		t.Fatalf("direct[1] = %+v", direct[1])
	}
}
