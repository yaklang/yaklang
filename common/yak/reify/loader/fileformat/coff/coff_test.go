package coff

import (
	"context"
	"encoding/binary"
	"io"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// object builds one COFF object: a text section with two bytes and one
// function symbol.
func object() []byte {
	const headerSize = 20
	const sectionSize = 40
	const symbolSize = 18
	const rawOffset = 128
	const symbolOffset = rawOffset + 4
	fileHeader := make([]byte, headerSize)
	binary.LittleEndian.PutUint16(fileHeader[0:], machineAMD64)
	binary.LittleEndian.PutUint16(fileHeader[2:], 1) // one section
	binary.LittleEndian.PutUint32(fileHeader[8:], symbolOffset)
	binary.LittleEndian.PutUint32(fileHeader[12:], 1) // one symbol
	binary.LittleEndian.PutUint16(fileHeader[16:], 0) // no optional header

	section := make([]byte, sectionSize)
	copy(section, ".text\x00\x00\x00")
	binary.LittleEndian.PutUint32(section[8:], 4)  // virtual size
	binary.LittleEndian.PutUint32(section[12:], 0) // virtual address
	binary.LittleEndian.PutUint32(section[16:], 4) // raw size
	binary.LittleEndian.PutUint32(section[20:], rawOffset)
	binary.LittleEndian.PutUint32(section[36:], sectionCode|sectionExecute)

	symbol := make([]byte, symbolSize)
	copy(symbol, "main\x00\x00\x00\x00")
	binary.LittleEndian.PutUint32(symbol[8:], 0)  // value
	binary.LittleEndian.PutUint16(symbol[12:], 1) // section number
	symbol[16] = 2                                // external storage class

	result := make([]byte, 0, symbolOffset+symbolSize)
	result = append(result, fileHeader...)
	result = append(result, section...)
	// Pad up to the raw section data.
	for len(result) < rawOffset {
		result = append(result, 0)
	}
	result = append(result, 0x90, 0x90, 0xc3, 0x00)
	result = append(result, symbol...)
	return result
}

func readerFor(t *testing.T, data []byte) *core.ByteReader {
	t.Helper()
	reader, err := core.NewByteReader("test", readerAt(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

// longNameObject builds one COFF object whose only function name does not fit in
// the eight inline bytes, so the entry stores a string table offset instead.
func longNameObject() []byte {
	const headerSize = 20
	const sectionSize = 40
	const rawOffset = 128
	name := "a_very_long_function_name"
	stringTable := make([]byte, 4+len(name)+1)
	binary.LittleEndian.PutUint32(stringTable, uint32(len(stringTable)))
	copy(stringTable[4:], name)
	symbolOffset := rawOffset + 4
	fileHeader := make([]byte, headerSize)
	binary.LittleEndian.PutUint16(fileHeader[0:], machineAMD64)
	binary.LittleEndian.PutUint16(fileHeader[2:], 1)
	binary.LittleEndian.PutUint32(fileHeader[8:], uint32(symbolOffset))
	binary.LittleEndian.PutUint32(fileHeader[12:], 1)
	section := make([]byte, sectionSize)
	copy(section, ".text\x00\x00\x00")
	binary.LittleEndian.PutUint32(section[8:], 4)
	binary.LittleEndian.PutUint32(section[16:], 4)
	binary.LittleEndian.PutUint32(section[20:], rawOffset)
	binary.LittleEndian.PutUint32(section[36:], sectionCode|sectionExecute)
	symbol := make([]byte, symbolSize)
	// A zero first word marks the string table form; the offset starts at four.
	binary.LittleEndian.PutUint32(symbol[4:], 4)
	binary.LittleEndian.PutUint32(symbol[8:], 0)
	binary.LittleEndian.PutUint16(symbol[12:], 1)
	binary.LittleEndian.PutUint16(symbol[14:], symbolTypeFunction)
	symbol[16] = 2 // external storage class

	result := make([]byte, 0, symbolOffset+symbolSize+len(stringTable))
	result = append(result, fileHeader...)
	result = append(result, section...)
	for len(result) < rawOffset {
		result = append(result, 0)
	}
	result = append(result, 0x90, 0x90, 0xc3, 0x00)
	result = append(result, symbol...)
	result = append(result, stringTable...)
	return result
}

type readerAt []byte

func (r readerAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r)) {
		return 0, io.EOF
	}
	n := copy(p, r[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func TestMatch(t *testing.T) {
	if !(Loader{}).Match(object()) {
		t.Fatal("COFF object not accepted")
	}
	// A Mach-O or ELF header must not match.
	elf := []byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0}
	elf = append(elf, make([]byte, 16)...)
	if (Loader{}).Match(elf) {
		t.Fatal("ELF accepted by the COFF loader")
	}
}

func TestLoad(t *testing.T) {
	artifact, err := Loader{}.Load(context.Background(), readerFor(t, object()))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Target.Architecture != core.ArchAMD64 || artifact.Target.Bits != 64 {
		t.Fatalf("target = %+v", artifact.Target)
	}
	if artifact.Target.OS != core.OSWindows {
		t.Fatalf("os = %q", artifact.Target.OS)
	}
	if len(artifact.Image.Sections) != 1 {
		t.Fatalf("sections = %d", len(artifact.Image.Sections))
	}
	if artifact.Image.Sections[0].Name != ".text" {
		t.Fatalf("section = %q", artifact.Image.Sections[0].Name)
	}
	if len(artifact.Hints.ExecSections) != 1 {
		t.Fatalf("exec sections = %d", len(artifact.Hints.ExecSections))
	}
	if len(artifact.Image.Symbols) != 1 || artifact.Image.Symbols[0].Name != "main" {
		t.Fatalf("symbols = %+v", artifact.Image.Symbols)
	}
	// The first function of a section sits at offset zero, which is a real
	// address in a relocatable object.
	if len(artifact.Hints.FunctionStarts) != 1 || artifact.Hints.FunctionStarts[0] != 0 {
		t.Fatalf("function starts = %v", artifact.Hints.FunctionStarts)
	}
}

// TestLoadLongSymbolName checks a name that does not fit in the inline field.
func TestLoadLongSymbolName(t *testing.T) {
	artifact, err := Loader{}.Load(context.Background(), readerFor(t, longNameObject()))
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Image.Symbols) != 1 {
		t.Fatalf("symbols = %+v", artifact.Image.Symbols)
	}
	if got := artifact.Image.Symbols[0].Name; got != "a_very_long_function_name" {
		t.Fatalf("name = %q", got)
	}
	if got := artifact.Image.Symbols[0].Kind; got != sym.SymbolFunction {
		t.Fatalf("kind = %v", got)
	}
}

func TestLoadRejectsIncompleteSymbolRecord(t *testing.T) {
	data := object()
	_, err := Loader{}.Load(context.Background(), readerFor(t, data[:len(data)-1]))
	if err == nil || !strings.Contains(err.Error(), "coff: invalid symbol table") {
		t.Fatalf("expected a symbol table error, got %v", err)
	}
}

// TestLoadLaysSectionsOut checks that sections sharing the default address zero
// get distinct bases. A relocatable object has no assigned addresses, so without
// the layout every section would overlap.
func TestLoadLaysSectionsOut(t *testing.T) {
	raw := object()
	binary.LittleEndian.PutUint16(raw[2:], 2)
	// Shift the section table by one header and move the payload down.
	section := raw[20 : 20+40]
	second := make([]byte, 40)
	copy(second, section)
	copy(second, ".data\x00\x00\x00")
	binary.LittleEndian.PutUint32(second[12:], 0)
	binary.LittleEndian.PutUint32(second[16:], 4)
	binary.LittleEndian.PutUint32(second[20:], 132)
	binary.LittleEndian.PutUint32(second[36:], 0x40000040)
	// Move the symbol table past the extra header.
	binary.LittleEndian.PutUint32(raw[8:], 150)
	rebuilt := make([]byte, 0, len(raw)+40)
	rebuilt = append(rebuilt, raw[:60]...)
	rebuilt = append(rebuilt, second...)
	rebuilt = append(rebuilt, raw[68:]...)

	artifact, err := Loader{}.Load(context.Background(), readerFor(t, rebuilt))
	if err != nil {
		t.Fatal(err)
	}
	if len(artifact.Image.Sections) != 2 {
		t.Fatalf("sections = %d", len(artifact.Image.Sections))
	}
	first := artifact.Image.Sections[0].VirtualAddress
	other := artifact.Image.Sections[1].VirtualAddress
	if first == other {
		t.Fatalf("both sections start at %#x", first)
	}
	if other < first+artifact.Image.Sections[0].MemorySize {
		t.Fatalf("sections overlap: %#x and %#x", first, other)
	}
}
