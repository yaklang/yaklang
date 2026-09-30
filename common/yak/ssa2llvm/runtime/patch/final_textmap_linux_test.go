//go:build linux

package patch

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// TestSortFinalTextMapUsesModuleSymbol covers a linked binary whose
// .go.module section starts before runtime.firstmoduledata, and whose slice
// header still points at a decoy table. Sorting must follow the symbol and
// the runtime.textsectionmap size, then order entries by baseaddr.
func TestSortFinalTextMapUsesModuleSymbol(t *testing.T) {
	const (
		pad      = 0x100
		moduleVA = 0x400000
		dataVA   = 0x500000
		textVA   = 0x600000
		etextVA  = 0x601000
		mapOff   = 0
		decoyOff = 72
		ftabOff  = 160
		mapLen   = 3
		decoyEnd = uint64(0x25a6c0000cc620)
	)
	moduleSize := pad + 0x200
	dataSize := 256

	module := make([]byte, moduleSize)
	md := pad
	putU64 := func(b []byte, off int, v uint64) {
		binary.LittleEndian.PutUint64(b[off:], v)
	}
	// A reader that starts at the section sees this header and would treat
	// the decoy as the text map.
	putU64(module, 0x168, dataVA+decoyOff)
	putU64(module, 0x170, 2)
	putU64(module, 0x178, 2)
	putU64(module, md+moduledataFtabOff, dataVA+ftabOff)
	putU64(module, md+moduledataFtabLen, 2)
	putU64(module, md+moduledataFtabOff+16, 2)
	putU64(module, md+moduledataTextOff, textVA)
	putU64(module, md+moduledataEtextOff, etextVA)
	// The real header is also wrong until the textsectionmap symbol rewrites it.
	putU64(module, md+moduledataTextsectMapOff, dataVA+decoyOff)
	putU64(module, md+moduledataTextsectMapLen, 2)
	putU64(module, md+moduledataTextsectMapCap, 2)

	data := make([]byte, dataSize)
	// Unsorted real map. The last entry is the etext sentinel.
	entries := [][3]uint64{
		{0x10, 0x20, 0x2000},
		{0x30, 0x40, 0x1000},
		{0x3f, 0x40, etextVA},
	}
	for i, e := range entries {
		base := mapOff + i*textsectEntrySize
		putU64(data, base, e[0])
		putU64(data, base+8, e[1])
		putU64(data, base+16, e[2])
	}
	putU64(data, decoyOff+8, decoyEnd)

	raw := buildTextMapELF(t, moduleVA, dataVA, module, data)
	dir := t.TempDir()
	path := filepath.Join(dir, "a.out")
	if err := os.WriteFile(path, raw, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SortFinalTextMap(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dataFile := 64 + len(module)
	mdFile := 64 + pad
	if binary.LittleEndian.Uint64(got[mdFile+moduledataTextsectMapOff:]) != dataVA {
		t.Fatalf("header ptr %#x", binary.LittleEndian.Uint64(got[mdFile+moduledataTextsectMapOff:]))
	}
	if binary.LittleEndian.Uint64(got[mdFile+moduledataTextsectMapLen:]) != mapLen {
		t.Fatalf("header len %d", binary.LittleEndian.Uint64(got[mdFile+moduledataTextsectMapLen:]))
	}
	wantBase := []uint64{0x1000, 0x2000, etextVA}
	for i, base := range wantBase {
		off := dataFile + i*textsectEntrySize + 16
		if gotBase := binary.LittleEndian.Uint64(got[off:]); gotBase != base {
			t.Fatalf("entry %d baseaddr %#x, want %#x", i, gotBase, base)
		}
	}
	ftabSentinel := dataFile + ftabOff + functabEntrySize
	if entry := binary.LittleEndian.Uint32(got[ftabSentinel:]); entry != 0x3f {
		t.Fatalf("ftab sentinel entryoff %#x", entry)
	}
}

func buildTextMapELF(t *testing.T, moduleVA, dataVA uint64, module, data []byte) []byte {
	t.Helper()
	shstr := "\x00.go.module\x00.data\x00.strtab\x00.symtab\x00.shstrtab\x00"
	strtab := "\x00runtime.firstmoduledata\x00runtime.textsectionmap\x00"
	sym := make([]byte, elf64SymSize*3)
	putSym := func(i int, nameOff uint32, shndx uint16, value, size uint64) {
		off := i * elf64SymSize
		binary.LittleEndian.PutUint32(sym[off:], nameOff)
		sym[off+4] = byte(elfSTBGlobal)<<4 | byte(elfSTTObject)
		binary.LittleEndian.PutUint16(sym[off+6:], shndx)
		binary.LittleEndian.PutUint64(sym[off+8:], value)
		binary.LittleEndian.PutUint64(sym[off+16:], size)
	}
	putSym(1, 1, 1, moduleVA+0x100, 0x200)
	putSym(2, 1+uint32(len("runtime.firstmoduledata"))+1, 2, dataVA, 3*textsectEntrySize)

	chunks := [][]byte{module, data, []byte(strtab), sym, []byte(shstr)}
	off := 64
	offsets := make([]int, len(chunks))
	for i, c := range chunks {
		offsets[i] = off
		off += len(c)
	}
	shoff := off
	nsec := 6
	raw := make([]byte, shoff+nsec*64)
	copy(raw[0:], []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(raw[16:], 2) // ET_EXEC
	binary.LittleEndian.PutUint16(raw[18:], 62)
	binary.LittleEndian.PutUint32(raw[20:], 1)
	binary.LittleEndian.PutUint64(raw[40:], uint64(shoff))
	binary.LittleEndian.PutUint16(raw[52:], 64)
	binary.LittleEndian.PutUint16(raw[58:], 64)
	binary.LittleEndian.PutUint16(raw[60:], uint16(nsec))
	binary.LittleEndian.PutUint16(raw[62:], 5) // shstrndx
	for i, c := range chunks {
		copy(raw[offsets[i]:], c)
	}
	names := []uint32{0, 1, 12, 18, 26, 34}
	types := []uint32{0, 1, 1, 3, 2, 3} // NULL, PROGBITS, PROGBITS, STRTAB, SYMTAB, STRTAB
	flags := []uint64{0, 3, 3, 0, 0, 0}
	addrs := []uint64{0, moduleVA, dataVA, 0, 0, 0}
	links := []uint32{0, 0, 0, 0, 3, 0}
	sizes := []int{0, len(module), len(data), len(strtab), len(sym), len(shstr)}
	fileOff := []int{0, offsets[0], offsets[1], offsets[2], offsets[3], offsets[4]}
	for i := 0; i < nsec; i++ {
		hdr := shoff + i*64
		binary.LittleEndian.PutUint32(raw[hdr:], names[i])
		binary.LittleEndian.PutUint32(raw[hdr+4:], types[i])
		binary.LittleEndian.PutUint64(raw[hdr+8:], flags[i])
		binary.LittleEndian.PutUint64(raw[hdr+16:], addrs[i])
		binary.LittleEndian.PutUint64(raw[hdr+24:], uint64(fileOff[i]))
		binary.LittleEndian.PutUint64(raw[hdr+32:], uint64(sizes[i]))
		binary.LittleEndian.PutUint32(raw[hdr+40:], links[i])
	}
	return raw
}

const (
	elfSTBGlobal = 1
	elfSTTObject = 1
)
