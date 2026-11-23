package golang

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

const (
	testPclnAddress   = 0x1000
	testModuleAddress = 0x2000
	testTextAddress   = 0x3000
	testETextAddress  = 0x3100
)

// moduleArtifact builds a minimal Go 1.20 table header and one module data
// structure. The table header has no functions; only the header words that
// validation reads are meaningful.
func moduleArtifact(t *testing.T, moduleSectionName string, mutate func([]byte)) *core.Artifact {
	t.Helper()
	pcln := make([]byte, 0x100)
	binary.LittleEndian.PutUint32(pcln, 0xfffffff1)
	pcln[6], pcln[7] = 1, 8
	putWord := func(offset int, value uint64) { binary.LittleEndian.PutUint64(pcln[offset:], value) }
	putWord(8, 3)     // nfunc
	putWord(16, 1)    // nfiles
	putWord(24, 0)    // unused
	putWord(32, 0x48) // funcnametab
	putWord(40, 0x60) // cutab
	putWord(48, 0x80) // filetab
	putWord(56, 0xa0) // pctab
	putWord(64, 0xc0) // pclntable and ftab

	module := make([]byte, moduleDataMinimumSize)
	mput := func(offset int, value uint64) { binary.LittleEndian.PutUint64(module[offset:], value) }
	mput(moduleDataHeaderOffset, testPclnAddress)
	mput(moduleDataFuncNameOffset, testPclnAddress+0x48)
	mput(moduleDataCUTabOffset, testPclnAddress+0x60)
	mput(moduleDataFileTabOffset, testPclnAddress+0x80)
	mput(moduleDataPCTabOffset, testPclnAddress+0xa0)
	mput(moduleDataPCLNTableOffset, testPclnAddress+0xc0)
	mput(moduleDataFTabOffset, testPclnAddress+0xc0)
	mput(moduleDataFTabLenOffset, 4)
	mput(moduleDataMinPCOffset, testTextAddress)
	mput(moduleDataMaxPCOffset, testETextAddress-0x10)
	mput(moduleDataTextOffset, testTextAddress)
	mput(moduleDataETextOffset, testETextAddress)
	if mutate != nil {
		mutate(module)
	}

	data := make([]byte, testModuleAddress-testPclnAddress+len(module))
	copy(data, pcln)
	copy(data[testModuleAddress-testPclnAddress:], module)
	reader, err := core.NewByteReader("module", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return &core.Artifact{
		Kind:   core.KindNative,
		Format: core.FormatELF,
		Target: core.Target{Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian},
		Reader: reader,
		Image: &core.Image{
			Sections: []core.Section{
				{Name: ".gopclntab", FileOffset: 0, FileSize: uint64(len(pcln)), VirtualAddress: testPclnAddress, MemorySize: uint64(len(pcln))},
				{Name: moduleSectionName, FileOffset: testModuleAddress - testPclnAddress, FileSize: uint64(len(module)),
					VirtualAddress: testModuleAddress, MemorySize: uint64(len(module)), Writable: true},
			},
		},
		Hints: &core.BoundaryHints{ExecSections: []core.Region{
			{Address: testTextAddress, Size: testETextAddress - testTextAddress},
		}},
	}
}

func testTables() pclnTables {
	word := func(index int) uint64 {
		switch index {
		case 3:
			return 0x48
		case 4:
			return 0x60
		case 5:
			return 0x80
		case 6:
			return 0xa0
		case 7:
			return 0xc0
		}
		return 0
	}
	return readPclnTables(testPclnAddress, 3, word)
}

func TestRecoverModuleDataFromNamedSection(t *testing.T) {
	a := moduleArtifact(t, ".go.module", nil)
	found, ok := recoverModuleData(a, testTables(), binary.LittleEndian)
	if !ok {
		t.Fatal("module data not recovered")
	}
	if found.Text != testTextAddress || found.EText != testETextAddress ||
		found.FuncCount != 3 || found.Address != testModuleAddress {
		t.Fatalf("module data = %+v", found)
	}
	if found.Source != "scan .go.module" {
		t.Fatalf("source = %q", found.Source)
	}
}

func TestRecoverModuleDataScansDataSection(t *testing.T) {
	a := moduleArtifact(t, ".noptrdata", nil)
	if _, ok := recoverModuleData(a, testTables(), binary.LittleEndian); !ok {
		t.Fatal("module data in .noptrdata was not found")
	}
}

func TestRecoverModuleDataRejectsMismatch(t *testing.T) {
	for name, mutate := range map[string]func([]byte){
		"header pointer":   func(data []byte) { binary.LittleEndian.PutUint64(data[moduleDataHeaderOffset:], 0x9999) },
		"file table":       func(data []byte) { binary.LittleEndian.PutUint64(data[moduleDataFileTabOffset:], 0x9999) },
		"function count":   func(data []byte) { binary.LittleEndian.PutUint64(data[moduleDataFTabLenOffset:], 99) },
		"text above etext": func(data []byte) { binary.LittleEndian.PutUint64(data[moduleDataTextOffset:], testETextAddress+1) },
		"maxpc outside":    func(data []byte) { binary.LittleEndian.PutUint64(data[moduleDataMaxPCOffset:], testETextAddress) },
	} {
		t.Run(name, func(t *testing.T) {
			a := moduleArtifact(t, ".go.module", mutate)
			if _, ok := recoverModuleData(a, testTables(), binary.LittleEndian); ok {
				t.Fatal("candidate accepted")
			}
		})
	}
}

func TestTextStartPrefersRuntimeTextSymbol(t *testing.T) {
	a := moduleArtifact(t, ".go.module", nil)
	a.Image.SetSymbols([]symbol.Symbol{{Name: "runtime.text", Address: 0x9000, Size: 1, Kind: symbol.SymbolFunction}})
	base, err := textStart(a, testTables(), binary.LittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	if base != 0x9000 {
		t.Fatalf("text base = 0x%x, want the runtime.text symbol", base)
	}
}

func TestTextStartUsesModuleDataWhenStripped(t *testing.T) {
	a := moduleArtifact(t, ".go.module", nil)
	base, err := textStart(a, testTables(), binary.LittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	if base != testTextAddress {
		t.Fatalf("text base = 0x%x, want module data", base)
	}
}

func TestTextStartFallsBackToTextSection(t *testing.T) {
	a := moduleArtifact(t, ".data", func(data []byte) {
		binary.LittleEndian.PutUint64(data[moduleDataHeaderOffset:], 0x9999)
	})
	a.Image.Sections = append(a.Image.Sections, core.Section{Name: ".text", VirtualAddress: 0x7000, FileSize: 0x100, MemorySize: 0x100})
	base, err := textStart(a, testTables(), binary.LittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	if base != 0x7000 {
		t.Fatalf("text base = 0x%x, want .text", base)
	}
}

func TestModuleDataLimitAllowsLargePERData(t *testing.T) {
	rdata := core.Section{Name: ".rdata", FileSize: 70 << 20}
	if got := moduleDataLimit(rdata, core.FormatPE); got != maxNamedPclntabSize {
		t.Fatalf("PE .rdata scan limit = %d", got)
	}
	if got := moduleDataLimit(rdata, core.FormatELF); got != maxMetadataSize {
		t.Fatalf("ELF .rdata scan limit = %d", got)
	}
}

func TestModuleDataSourceSectionNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{".noptrdata", true},
		{".data.rel.ro", true},
		{".rdata", true},
		{"__DATA,__data", true},
		{"__DATA_CONST,__const", true},
		{"__DATA,__bss", true},
		{"__TEXT,__text", false},
		{"__TEXT,__gopclntab", false},
		{".gopclntab", false},
	} {
		if got := moduleDataSourceSection(core.Section{Name: tc.name}); got != tc.want {
			t.Errorf("moduleDataSourceSection(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTextStartFallsBackToMachOTextSection(t *testing.T) {
	a := &core.Artifact{Image: &core.Image{Sections: []core.Section{
		{Name: "__TEXT,__text", VirtualAddress: 0x8000, FileSize: 0x100, MemorySize: 0x100},
	}}}
	base, err := textStart(a, pclnTables{}, binary.LittleEndian)
	if err != nil {
		t.Fatal(err)
	}
	if base != 0x8000 {
		t.Fatalf("text base = 0x%x, want __TEXT,__text", base)
	}
}
