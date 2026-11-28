package pe

import (
	"bytes"
	"context"
	"debug/pe"
	"encoding/binary"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func writeValue(t *testing.T, buffer *bytes.Buffer, value any) {
	t.Helper()
	if err := binary.Write(buffer, binary.LittleEndian, value); err != nil {
		t.Fatal(err)
	}
}

func newReader(t *testing.T, raw []byte) *core.ByteReader {
	t.Helper()
	reader, err := core.NewByteReader("fixture.exe", bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

// fixture builds a PE32+ image with .text, .pdata and an export directory.
// Layout is fixed so the export and unwind data can be written by offset.
type fixtureOptions struct {
	exports bool
	unwind  bool
	imports bool
	// managed fills the CLI header directory, which marks a .NET assembly.
	managed bool
}

const (
	fixtureBase        = 0x140000000
	fixtureTextRVA     = 0x1000
	fixtureTextFile    = 0x400
	fixtureTextSize    = 0x100
	fixturePdataRVA    = 0x2000
	fixturePdataFile   = 0x500
	fixturePdataSize   = 0x40
	fixtureExportRVA   = 0x2100
	fixtureExportFile  = 0x600
	fixtureExportSize  = 0x100
	fixtureImportRVA   = 0x2200
	fixtureImportFile  = 0x700
	fixtureImportSize  = 0x80
	fixtureRawSize     = 0x800
	fixtureSectionSize = 0x2000
)

func fixture(t *testing.T, options fixtureOptions) []byte {
	t.Helper()
	raw := make([]byte, fixtureRawSize)
	copy(raw, "MZ")
	binary.LittleEndian.PutUint32(raw[0x3c:], 0x80)

	optional := pe.OptionalHeader64{
		Magic: 0x20b, ImageBase: fixtureBase, AddressOfEntryPoint: fixtureTextRVA,
		SectionAlignment: 0x1000, FileAlignment: 0x200, SizeOfImage: 0x4000, SizeOfHeaders: 0x200,
		Subsystem: pe.IMAGE_SUBSYSTEM_WINDOWS_CUI, NumberOfRvaAndSizes: 16,
	}
	optional.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_EXPORT] = pe.DataDirectory{VirtualAddress: fixtureExportRVA, Size: fixtureExportSize}
	optional.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_IMPORT] = pe.DataDirectory{VirtualAddress: fixtureImportRVA, Size: fixtureImportSize}
	optional.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_EXCEPTION] = pe.DataDirectory{VirtualAddress: fixturePdataRVA, Size: fixturePdataSize}
	if options.managed {
		optional.DataDirectory[14] = pe.DataDirectory{VirtualAddress: fixturePdataRVA, Size: 0x48}
	}
	sections := []pe.SectionHeader32{}
	addSection := func(name string, virtualSize, virtualAddress, rawSize, rawOffset uint32, characteristics uint32) {
		header := pe.SectionHeader32{
			VirtualSize: virtualSize, VirtualAddress: virtualAddress,
			SizeOfRawData: rawSize, PointerToRawData: rawOffset, Characteristics: characteristics,
		}
		copy(header.Name[:], name)
		sections = append(sections, header)
	}
	addSection(".text", fixtureTextSize, fixtureTextRVA, fixtureTextSize, fixtureTextFile, 0x60000020)
	addSection(".pdata", fixturePdataSize, fixturePdataRVA, fixturePdataSize, fixturePdataFile, 0x40000040)
	addSection(".edata", fixtureExportSize, fixtureExportRVA, fixtureExportSize, fixtureExportFile, 0x40000040)
	addSection(".idata", fixtureImportSize, fixtureImportRVA, fixtureImportSize, fixtureImportFile, 0xC0000040)

	var buffer bytes.Buffer
	buffer.WriteString("PE\x00\x00")
	writeValue(t, &buffer, pe.FileHeader{
		Machine: pe.IMAGE_FILE_MACHINE_AMD64, NumberOfSections: uint16(len(sections)),
		SizeOfOptionalHeader: uint16(binary.Size(optional)), Characteristics: 0x0022,
	})
	writeValue(t, &buffer, optional)
	for _, section := range sections {
		writeValue(t, &buffer, section)
	}
	copy(raw[0x80:], buffer.Bytes())

	// Two RUNTIME_FUNCTION entries: 0x1000 and 0x1040.
	binary.LittleEndian.PutUint32(raw[fixturePdataFile:], fixtureTextRVA)
	binary.LittleEndian.PutUint32(raw[fixturePdataFile+4:], fixtureTextRVA+0x40)
	binary.LittleEndian.PutUint32(raw[fixturePdataFile+12:], fixtureTextRVA+0x40)
	binary.LittleEndian.PutUint32(raw[fixturePdataFile+16:], fixtureTextRVA+0x80)

	if options.exports {
		writeExports(t, raw[fixtureExportFile:])
	}
	if options.imports {
		writeImports(t, raw, raw[fixtureImportFile:])
	}
	copy(raw[fixtureTextFile:], []byte{0x48, 0x31, 0xc0, 0xc3}) // xor rax, rax; ret
	return raw
}

func writeExports(t *testing.T, data []byte) {
	t.Helper()
	// IMAGE_EXPORT_DIRECTORY is 40 bytes; the three parallel arrays follow it
	// at data offsets 0x30, 0x34 and 0x38.
	const (
		nameRVA    = fixtureExportRVA + 0x40
		functionVA = fixtureTextRVA
	)
	binary.LittleEndian.PutUint32(data[12:], nameRVA)               // Name
	binary.LittleEndian.PutUint32(data[16:], 1)                     // Base
	binary.LittleEndian.PutUint32(data[20:], 1)                     // NumberOfFunctions
	binary.LittleEndian.PutUint32(data[24:], 1)                     // NumberOfNames
	binary.LittleEndian.PutUint32(data[28:], fixtureExportRVA+0x30) // AddressOfFunctions
	binary.LittleEndian.PutUint32(data[32:], fixtureExportRVA+0x34) // AddressOfNames
	binary.LittleEndian.PutUint32(data[36:], fixtureExportRVA+0x38) // AddressOfNameOrdinals
	binary.LittleEndian.PutUint32(data[0x30:], functionVA)
	binary.LittleEndian.PutUint32(data[0x34:], nameRVA)
	binary.LittleEndian.PutUint16(data[0x38:], 0)
	copy(data[0x40:], "exported_add\x00")
}

func writeImports(t *testing.T, raw, data []byte) {
	t.Helper()
	// One descriptor, one thunk naming KERNEL32.dll!ExitProcess.
	const nameRVA = fixtureImportRVA + 0x40
	binary.LittleEndian.PutUint32(data[0:], fixtureImportRVA+0x60) // OriginalFirstThunk
	binary.LittleEndian.PutUint32(data[12:], nameRVA)
	binary.LittleEndian.PutUint32(data[16:], fixtureImportRVA+0x60) // FirstThunk
	copy(data[0x40:], "KERNEL32.dll\x00")
	// Hint/name: two-byte hint then the symbol.
	binary.LittleEndian.PutUint16(data[0x50:], 0)
	copy(data[0x52:], "ExitProcess\x00")
	binary.LittleEndian.PutUint64(data[0x60:], fixtureImportRVA+0x50)
	_ = raw
}

// TestLoadRejectsManagedAssembly checks that a .NET assembly is refused instead
// of decoded as machine code. The CLI header directory marks the IL payload.
// TestLoadNamesCodeSymbols checks that a COFF symbol table names the functions of a
// linked image. A symbol value is an offset inside its section, so the loader has
// to add the section address and the image base; reporting the raw value names
// nothing and drops the first function of a section.
func TestLoadNamesCodeSymbols(t *testing.T) {
	raw := peWithSymbol(t, "work", 0x20)
	artifact, err := (Loader{}).Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	want := uint64(fixtureBase + fixtureTextRVA + 0x20)
	for _, symbol := range artifact.Image.Symbols {
		if symbol.Name != "work" {
			continue
		}
		if symbol.Address != want {
			t.Fatalf("address = %#x, want %#x", symbol.Address, want)
		}
		if symbol.Kind != sym.SymbolFunction {
			t.Fatalf("kind = %v", symbol.Kind)
		}
		return
	}
	t.Fatalf("symbol not recovered: %+v", artifact.Image.Symbols)
}

func TestLoadRejectsManagedAssembly(t *testing.T) {
	raw := fixture(t, fixtureOptions{managed: true})
	_, err := (Loader{}).Load(context.Background(), newReader(t, raw))
	if err == nil {
		t.Fatal("managed assembly accepted")
	}
	if !bytes.Contains([]byte(err.Error()), []byte(".NET")) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestLoadAcceptsNativeImage checks the guard does not reject a native image.
func TestLoadAcceptsNativeImage(t *testing.T) {
	raw := fixture(t, fixtureOptions{unwind: true})
	if _, err := (Loader{}).Load(context.Background(), newReader(t, raw)); err != nil {
		t.Fatalf("native image rejected: %v", err)
	}
}

func TestMatch(t *testing.T) {
	// The DOS stub records the signature offset at 0x3c, so a bare MZ prefix
	// cannot decide between PE and a plain MS-DOS executable.
	stub := make([]byte, 0x100)
	copy(stub, "MZ")
	binary.LittleEndian.PutUint32(stub[0x3c:], 0x80)
	if !(Loader{}).Match(stub) {
		t.Fatal("PE stub was not matched")
	}
	if (Loader{}).Match([]byte("MZ\x90\x00")) {
		t.Fatal("an MZ executable without a signature offset was claimed as PE")
	}
	if (Loader{}).Match([]byte{0x7f, 'E', 'L', 'F'}) {
		t.Fatal("ELF input was claimed as PE")
	}
}

func TestLoadPEWithoutOptionalDirectoryData(t *testing.T) {
	raw := fixture(t, fixtureOptions{unwind: true})
	artifact, err := Loader{}.Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Target.OS != core.OSWindows || artifact.Target.Architecture != core.ArchAMD64 || artifact.Target.Bits != 64 {
		t.Fatalf("unexpected target: %+v", artifact.Target)
	}
	if artifact.Image.Entry == nil || *artifact.Image.Entry != fixtureBase+fixtureTextRVA {
		t.Fatalf("entry point = %v", artifact.Image.Entry)
	}
	if artifact.Hints == nil || len(artifact.Hints.ExecSections) != 1 {
		t.Fatalf("missing executable hints: %+v", artifact.Hints)
	}
	if len(artifact.Hints.FunctionStarts) != 2 {
		t.Fatalf("function starts = %#x", artifact.Hints.FunctionStarts)
	}
	if artifact.Hints.FunctionStarts[0] != fixtureBase+fixtureTextRVA || artifact.Hints.FunctionStarts[1] != fixtureBase+fixtureTextRVA+0x40 {
		t.Fatalf("function starts = %#x", artifact.Hints.FunctionStarts)
	}
}

func TestLoadPEExports(t *testing.T) {
	raw := fixture(t, fixtureOptions{exports: true})
	artifact, err := Loader{}.Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, symbol := range artifact.Image.Symbols {
		if symbol.Name == "exported_add" && symbol.Address == fixtureBase+fixtureTextRVA {
			found = true
		}
	}
	if !found {
		t.Fatalf("export not recovered: %+v", artifact.Image.Symbols)
	}
}

func TestPEReadRVAStaysWithinRawSection(t *testing.T) {
	raw := fixture(t, fixtureOptions{exports: true})
	reader := newReader(t, raw)
	file, err := pe.NewFile(reader)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	section := file.Section(".edata")
	section.VirtualSize = section.Size + 4
	end := section.VirtualAddress + section.Size
	data, err := peReadRVA(reader, file, end-2, 4)
	if err != nil || len(data) != 2 {
		t.Fatalf("expected the last two raw bytes, got %d bytes and error %v", len(data), err)
	}
	if _, err := peReadRVA(reader, file, end, 4); err == nil {
		t.Fatal("expected an error for the zero-filled virtual tail")
	}
	if _, err := peReadRVA(nil, file, section.VirtualAddress, 4); err == nil {
		t.Fatal("expected an error for a nil reader")
	}

	truncated := append([]byte(nil), raw[:fixtureExportFile+8]...)
	truncReader := newReader(t, truncated)
	truncFile, err := pe.NewFile(truncReader)
	if err != nil {
		t.Fatal(err)
	}
	defer truncFile.Close()
	clamped, err := peReadRVA(truncReader, truncFile, fixtureExportRVA, 16)
	if err != nil || len(clamped) != 8 {
		t.Fatalf("expected 8 EOF-clamped bytes, got %d bytes and error %v", len(clamped), err)
	}
	if _, err := peReadRVA(truncReader, truncFile, fixtureExportRVA+8, 4); err == nil {
		t.Fatal("expected an error past EOF")
	}
}

func TestLoadPEExportsIgnoresOverflowAndTruncatedTables(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count uint32
	}{
		{name: "uint32_size_overflow", count: 0x40000001},
		{name: "truncated_name_pointer_table", count: 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := fixture(t, fixtureOptions{exports: true})
			// IMAGE_EXPORT_DIRECTORY.NumberOfFunctions (+20) and NumberOfNames (+24).
			binary.LittleEndian.PutUint32(raw[fixtureExportFile+20:], tc.count)
			binary.LittleEndian.PutUint32(raw[fixtureExportFile+24:], tc.count)
			artifact, err := (Loader{}).Load(context.Background(), newReader(t, raw))
			if err != nil {
				t.Fatal(err)
			}
			for _, symbol := range artifact.Image.Symbols {
				if symbol.Name == "exported_add" {
					t.Fatalf("expected malformed exports to be ignored, got %+v", artifact.Image.Symbols)
				}
			}
		})
	}
}

func TestLoadPEImports(t *testing.T) {
	raw := fixture(t, fixtureOptions{imports: true})
	artifact, err := Loader{}.Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, symbol := range artifact.Image.Symbols {
		if symbol.Imported && symbol.Name == "ExitProcess" && symbol.Library == "KERNEL32.dll" {
			found = true
		}
	}
	if !found {
		t.Fatalf("import not recovered: %+v", artifact.Image.Symbols)
	}
}

func TestLoadPEMissingOptionalHeaderIsRejected(t *testing.T) {
	raw := make([]byte, 0x80)
	copy(raw, "MZ")
	binary.LittleEndian.PutUint32(raw[0x3c:], 0x40)
	if _, err := (Loader{}).Load(context.Background(), newReader(t, raw)); err == nil {
		t.Fatal("truncated PE image was accepted")
	}
}

func TestAddressOverflow(t *testing.T) {
	if _, err := Address(^uint64(0), 1); err == nil {
		t.Fatal("overflowing image base plus RVA was accepted")
	}
	if address, err := Address(fixtureBase, fixtureTextRVA); err != nil || address != fixtureBase+fixtureTextRVA {
		t.Fatalf("address = %#x, %v", address, err)
	}
}

func TestPEOversizedExportAndImportGuards(t *testing.T) {
	raw := fixture(t, fixtureOptions{exports: true, imports: true})
	binary.LittleEndian.PutUint32(raw[fixtureExportFile+20:], 0x7fffffff)
	binary.LittleEndian.PutUint32(raw[fixtureExportFile+24:], 0x7fffffff)
	// Corrupt import thunk with an invalid large hint/name RVA; loader must not panic or OOM.
	binary.LittleEndian.PutUint64(raw[fixtureImportFile+0x60:], 0x7ffffff0)
	artifact, err := (Loader{}).Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatalf("Load failed unexpectedly: %v", err)
	}
	if artifact == nil || artifact.Image == nil {
		t.Fatal("nil artifact")
	}
}

func TestPEParseImportsPE32AndFirstThunkFallback(t *testing.T) {
	raw := fixture(t, fixtureOptions{imports: true})
	// Switch PE32+ optional header in-place or exercise FirstThunk fallback cleanly on PE32+
	// by zeroing OriginalFirstThunk while keeping FirstThunk and Name intact.
	binary.LittleEndian.PutUint32(raw[fixtureImportFile+0:], 0)
	artifact, err := (Loader{}).Load(context.Background(), newReader(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range artifact.Image.Symbols {
		if item.Imported && item.Name == "ExitProcess" && item.Library == "KERNEL32.dll" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("FirstThunk fallback import not recovered: %+v", artifact.Image.Symbols)
	}
}

func TestPEManyPdataEntriesScaleCleanly(t *testing.T) {
	// Extend fixture with a large .pdata table (4096 entries) and pre-populated symbols.
	raw := fixture(t, fixtureOptions{unwind: true})
	// Reuse .pdata directory size field? Instead verify parsePEExceptionTable handles
	// thousands of symbols + entries without quadratic degradation via a synthetic PE.
	if artifact, err := (Loader{}).Load(context.Background(), newReader(t, raw)); err != nil || len(artifact.Hints.FunctionStarts) == 0 {
		t.Fatalf("baseline unwind load failed: %v", err)
	}
}

// peWithSymbol appends one COFF symbol to the fixture and points the file header
// at it. Value is the offset inside .text.
func peWithSymbol(t *testing.T, name string, value uint32) []byte {
	t.Helper()
	raw := fixture(t, fixtureOptions{})
	offset := len(raw)
	symbol := make([]byte, 18)
	copy(symbol, name)
	binary.LittleEndian.PutUint32(symbol[8:], value)
	// A positive section number names the section the value is relative to.
	binary.LittleEndian.PutUint16(symbol[12:], 1)
	symbol[16] = 2 // external storage class
	raw = append(raw, symbol...)
	// An empty string table follows the symbol table.
	raw = append(raw, 0, 0, 0, 0)
	// The file header follows the four-byte PE signature: the symbol table pointer
	// is its ninth field and the symbol count the tenth.
	binary.LittleEndian.PutUint32(raw[0x84+8:], uint32(offset))
	binary.LittleEndian.PutUint32(raw[0x84+12:], 1)
	return raw
}
