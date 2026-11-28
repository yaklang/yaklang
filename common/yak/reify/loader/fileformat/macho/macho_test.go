package macho

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/binary"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func writeValue(t *testing.T, buffer *bytes.Buffer, order binary.ByteOrder, value any) {
	t.Helper()
	if err := binary.Write(buffer, order, value); err != nil {
		t.Fatal(err)
	}
}

func newReader(t *testing.T, raw []byte) *core.ByteReader {
	t.Helper()
	reader, err := core.NewByteReader("fixture", bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

// thinFixture builds a 64-bit Mach-O with one executable section, LC_MAIN and
// LC_FUNCTION_STARTS. The three function starts are 0, 0x20 and 0x30 bytes
// after the section start.
func thinFixture(t *testing.T, cpu macho.Cpu, subCPU uint32) []byte {
	t.Helper()
	const (
		base        = 0x100000000
		textFile    = 0x400
		textSize    = 0x40
		startsFile  = 0x480
		startsSize  = 3
		entryOffset = 0x20
	)
	raw := make([]byte, 0x500)
	segment := macho.Segment64{
		Cmd: macho.LoadCmdSegment64, Len: 72 + 80, Addr: base, Memsz: textSize,
		Filesz: textSize, Maxprot: 7, Prot: 5, Nsect: 1,
	}
	copy(segment.Name[:], "__TEXT")
	text := macho.Section64{Addr: base + textFile, Size: textSize, Offset: textFile, Flags: 0x80000400}
	copy(text.Name[:], "__text")
	copy(text.Seg[:], "__TEXT")

	var buffer bytes.Buffer
	writeValue(t, &buffer, binary.LittleEndian, macho.FileHeader{
		Magic: macho.Magic64, Cpu: cpu, SubCpu: subCPU, Type: macho.TypeExec, Ncmd: 3,
		Cmdsz: segment.Len + 24 + 16,
	})
	writeValue(t, &buffer, binary.LittleEndian, uint32(0))
	writeValue(t, &buffer, binary.LittleEndian, segment)
	writeValue(t, &buffer, binary.LittleEndian, text)

	mainCommand := struct {
		Cmd      macho.LoadCmd
		Len      uint32
		EntryOff uint64
		Stack    uint64
	}{Cmd: machoCommandMain, Len: 24, EntryOff: textFile + entryOffset, Stack: 0}
	writeValue(t, &buffer, binary.LittleEndian, mainCommand)
	startsCommand := struct {
		Cmd     macho.LoadCmd
		Len     uint32
		DataOff uint32
		DataLen uint32
	}{Cmd: machoCommandFunctionStarts, Len: 16, DataOff: startsFile, DataLen: startsSize}
	writeValue(t, &buffer, binary.LittleEndian, startsCommand)
	copy(raw, buffer.Bytes())

	// Function starts are ULEB128 deltas from the __TEXT segment address.
	copy(raw[startsFile:], []byte{0x20, 0x10, 0x10})
	// mov eax, 1; ret
	copy(raw[textFile:], []byte{0xb8, 0x01, 0x00, 0x00, 0x00, 0xc3})
	return raw
}

func TestMatchAcceptsThinAndPlausibleFat(t *testing.T) {
	thin := thinFixture(t, macho.CpuArm64, 2)
	loader := Loader{}
	if !loader.Match(thin[:core.ProbeSize]) {
		t.Fatal("thin Mach-O was not matched")
	}
	fat := make([]byte, core.ProbeSize)
	binary.BigEndian.PutUint32(fat[0:], 0xcafebabe)
	binary.BigEndian.PutUint32(fat[4:], 2)
	binary.BigEndian.PutUint32(fat[8:], uint32(macho.CpuArm64))
	if !loader.Match(fat) {
		t.Fatal("plausible fat Mach-O was not matched")
	}
	// A Java class file shares the magic but has no plausible header.
	javaClass := make([]byte, core.ProbeSize)
	binary.BigEndian.PutUint32(javaClass[0:], 0xcafebabe)
	binary.BigEndian.PutUint32(javaClass[4:], 0x00000034)
	if loader.Match(javaClass) {
		t.Fatal("Java class file was claimed as Mach-O")
	}
	if loader.Match([]byte("MZ\x90\x00")) {
		t.Fatal("PE input was claimed as Mach-O")
	}
}

func TestLoadThinMachO(t *testing.T) {
	raw := thinFixture(t, macho.CpuArm64, 2)
	artifact, err := Loader{}.Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Target.OS != core.OSDarwin || artifact.Target.Architecture != core.ArchARM64 || artifact.Target.Bits != 64 {
		t.Fatalf("unexpected target: %+v", artifact.Target)
	}
	if artifact.Image.Entry == nil || *artifact.Image.Entry != 0x100000420 {
		t.Fatalf("entry point = %v", artifact.Image.Entry)
	}
	if artifact.Image.ObjectType != "Exec" {
		t.Fatalf("object type = %q", artifact.Image.ObjectType)
	}
	if len(artifact.Image.Sections) != 1 || artifact.Image.Sections[0].Name != "__TEXT,__text" {
		t.Fatalf("unexpected sections: %+v", artifact.Image.Sections)
	}
	if artifact.Hints == nil || len(artifact.Hints.ExecSections) != 1 {
		t.Fatalf("missing executable hints: %+v", artifact.Hints)
	}
	want := []uint64{0x100000020, 0x100000030, 0x100000040}
	if len(artifact.Hints.FunctionStarts) != len(want) {
		t.Fatalf("function starts = %#x", artifact.Hints.FunctionStarts)
	}
	for index, address := range want {
		if artifact.Hints.FunctionStarts[index] != address {
			t.Fatalf("function starts = %#x, want %#x", artifact.Hints.FunctionStarts, want)
		}
	}
}

func TestLoadThinMachOWithoutEntryReportsDiagnostic(t *testing.T) {
	raw := thinFixture(t, macho.CpuArm64, 2)
	// Overwrite LC_MAIN with an unrelated command so the entry is unavailable.
	binary.LittleEndian.PutUint32(raw[32+72+80:], 0x1b)
	artifact, err := Loader{}.Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Image.Entry != nil {
		t.Fatalf("entry point = %v", artifact.Image.Entry)
	}
	if len(artifact.Diagnostics) == 0 || artifact.Diagnostics[0].Code != "entry-point-unavailable" {
		t.Fatalf("missing diagnostic: %+v", artifact.Diagnostics)
	}
}

func fatFixture(t *testing.T, slices ...macho.Cpu) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writeValue(t, &buffer, binary.BigEndian, uint32(0xcafebabe))
	writeValue(t, &buffer, binary.BigEndian, uint32(len(slices)))
	type fatArch struct {
		CPU    uint32
		SubCPU uint32
		Offset uint32
		Size   uint32
		Align  uint32
	}
	headerSize := 8 + 20*len(slices)
	offset := uint32((headerSize + 15) &^ 15)
	archs := make([]fatArch, 0, len(slices))
	for _, cpu := range slices {
		thin := thinFixture(t, cpu, 0)
		archs = append(archs, fatArch{CPU: uint32(cpu), Offset: offset, Size: uint32(len(thin)), Align: 4})
		offset += uint32(len(thin))
	}
	for _, arch := range archs {
		writeValue(t, &buffer, binary.BigEndian, arch)
	}
	raw := make([]byte, offset)
	copy(raw, buffer.Bytes())
	cursor := uint32((headerSize + 15) &^ 15)
	for _, arch := range archs {
		thin := thinFixture(t, macho.Cpu(arch.CPU), 0)
		copy(raw[cursor:], thin)
		cursor += uint32(len(thin))
	}
	return raw
}

func TestLoadFatMachOPrefersARM64(t *testing.T) {
	raw := fatFixture(t, macho.CpuAmd64, macho.CpuArm64)
	artifact, err := Loader{}.Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Target.Architecture != core.ArchARM64 {
		t.Fatalf("selected %s, want arm64", artifact.Target.Architecture)
	}
	if len(artifact.Diagnostics) == 0 || artifact.Diagnostics[0].Code != "fat-slice-selected" {
		t.Fatalf("missing slice diagnostic: %+v", artifact.Diagnostics)
	}
	// Section offsets must be rebased onto the whole-file reader.
	for _, section := range artifact.Image.Sections {
		if section.FileOffset < 0x400 {
			t.Fatalf("section offset was not rebased: %+v", section)
		}
	}
}

func TestLoadFatMachOPrefersAMD64WhenNoARM64(t *testing.T) {
	raw := fatFixture(t, macho.Cpu386, macho.CpuAmd64)
	artifact, err := Loader{}.Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Target.Architecture != core.ArchAMD64 {
		t.Fatalf("selected %s, want amd64", artifact.Target.Architecture)
	}
}

func TestLoadMachOSymtabClassificationAndFiltering(t *testing.T) {
	const (
		textOffset  = 0x400
		constOffset = 0x420
		debugOffset = 0x440
		strOffset   = 0x500
		symOffset   = 0x600
	)
	segment := macho.Segment64{
		Cmd: macho.LoadCmdSegment64, Len: 72 + 5*80, Addr: 0, Memsz: 0x70,
		Offset: textOffset, Filesz: 0x50, Maxprot: 7, Prot: 5, Nsect: 5,
	}
	copy(segment.Name[:], "__TEXT")

	makeSection := func(seg, name string, addr, size uint64, offset, flags uint32) macho.Section64 {
		sec := macho.Section64{Addr: addr, Size: size, Offset: offset, Flags: flags}
		copy(sec.Seg[:], seg)
		copy(sec.Name[:], name)
		return sec
	}
	sections := []macho.Section64{
		makeSection("__TEXT", "__text", 0x00, 0x20, textOffset, machoAttrPureInstructions|machoAttrSomeInstructions),
		makeSection("__TEXT", "__const", 0x20, 0x20, constOffset, 0),
		makeSection("__DATA", "__bss", 0x40, 0x10, 0, 0x1),
		makeSection("__DWARF", "__debug_info", 0x50, 0x10, debugOffset, machoAttrDebug),
		makeSection("__TEXT", "__zeroexec", 0x60, 0x10, 0, 0x1|machoAttrPureInstructions),
	}

	var strTab bytes.Buffer
	strTab.WriteByte(0)
	addStr := func(name string) uint32 {
		off := uint32(strTab.Len())
		strTab.WriteString(name)
		strTab.WriteByte(0)
		return off
	}

	syms := []macho.Nlist64{
		{Name: addStr("_entry"), Type: machoSymbolSection | machoSymbolExternal, Sect: 1, Value: 0x00},
		{Name: addStr("ltmp0"), Type: machoSymbolSection, Sect: 1, Value: 0x00},
		{Name: addStr("_ltmp29"), Type: machoSymbolSection, Sect: 1, Value: 0x04},
		{Name: addStr("ltmp0"), Type: machoSymbolSection | machoSymbolExternal, Sect: 1, Value: 0x08},
		{Name: addStr("_greeting"), Type: machoSymbolSection | machoSymbolExternal, Sect: 2, Value: 0x20},
		{Name: addStr("_buffer"), Type: machoSymbolSection, Sect: 3, Value: 0x40},
		{Name: addStr("_zeroexec_sym"), Type: machoSymbolSection, Sect: 5, Value: 0x60},
		{Name: addStr("_stab_bnsym"), Type: 0x2e, Sect: 1, Value: 0x00},
		{Name: addStr("_stab_ensym"), Type: 0x4e, Sect: 1, Value: 0x04},
		{Name: addStr("_debug_sym"), Type: machoSymbolSection, Sect: 4, Value: 0x50},
		{Name: addStr("_out_of_range"), Type: machoSymbolSection, Sect: 1, Value: 0x20},
		{Name: addStr("_bad_sect"), Type: machoSymbolSection, Sect: 99, Value: 0x00},
	}

	var hdr bytes.Buffer
	writeValue(t, &hdr, binary.LittleEndian, macho.FileHeader{
		Magic: macho.Magic64, Cpu: macho.CpuArm64, SubCpu: 0, Type: macho.TypeObj,
		Ncmd: 2, Cmdsz: segment.Len + 24,
	})
	writeValue(t, &hdr, binary.LittleEndian, uint32(0))
	writeValue(t, &hdr, binary.LittleEndian, segment)
	for _, sec := range sections {
		writeValue(t, &hdr, binary.LittleEndian, sec)
	}
	writeValue(t, &hdr, binary.LittleEndian, macho.SymtabCmd{
		Cmd: macho.LoadCmdSymtab, Len: 24,
		Symoff: symOffset, Nsyms: uint32(len(syms)),
		Stroff: strOffset, Strsize: uint32(strTab.Len()),
	})

	var symBuf bytes.Buffer
	for _, item := range syms {
		writeValue(t, &symBuf, binary.LittleEndian, item)
	}

	raw := make([]byte, symOffset+symBuf.Len())
	copy(raw, hdr.Bytes())
	copy(raw[textOffset:], []byte{0xc0, 0x03, 0x5f, 0xd6})
	copy(raw[constOffset:], []byte("hello\x00"))
	copy(raw[strOffset:], strTab.Bytes())
	copy(raw[symOffset:], symBuf.Bytes())

	artifact, err := Loader{}.Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatal(err)
	}

	want := []sym.Symbol{
		{Name: "entry", Address: 0x00, Kind: sym.SymbolFunction},
		{Name: "ltmp29", Address: 0x04, Kind: sym.SymbolFunction},
		{Name: "ltmp0", Address: 0x08, Kind: sym.SymbolFunction},
		{Name: "greeting", Address: 0x20, Kind: sym.SymbolObject},
		{Name: "buffer", Address: 0x40, Kind: sym.SymbolObject},
		{Name: "zeroexec_sym", Address: 0x60, Kind: sym.SymbolObject},
	}
	if len(artifact.Image.Symbols) != len(want) {
		t.Fatalf("symbols = %+v, want %+v", artifact.Image.Symbols, want)
	}
	for i, expected := range want {
		got := artifact.Image.Symbols[i]
		if got.Name != expected.Name || got.Address != expected.Address || got.Kind != expected.Kind {
			t.Fatalf("symbol[%d] = %+v, want %+v", i, got, expected)
		}
	}
}
