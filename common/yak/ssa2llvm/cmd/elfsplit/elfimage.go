package main

// A minimal read/modify/write model of an ELF64 relocatable object.
//
// The module-splitting pass in main.go rewrites go.o by appending new payloads
// and repointing section headers at them, which keeps the 200 MB symbol and
// relocation tables in place. Function-name folding needs the opposite: a
// section has to get *smaller* and the file has to actually shrink, so the whole
// image is laid out again from parsed sections. Relocatable objects make that
// safe — there are no program headers and no section addresses to keep
// congruent, only per-section alignment.

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
)

const (
	elfHeaderSize = 64

	ehShoff     = 0x28
	ehShentsize = 0x3a
	ehShnum     = 0x3c
	ehShstrndx  = 0x3e

	shName      = 0
	shType      = 4
	shFlags     = 8
	shAddr      = 16
	shOffset    = 24
	shSize      = 32
	shLink      = 40
	shInfo      = 44
	shAddralign = 48
	shEntsize   = 56

	symName  = 0
	symInfo  = 4
	symOther = 5
	symShndx = 6
	symValue = 8
	symSize  = 16
)

type rawSection struct {
	name    string
	typ     uint32
	flags   uint64
	addr    uint64
	size    uint64
	link    uint32
	info    uint32
	align   uint64
	entsize uint64
	// payload is nil for SHT_NULL and SHT_NOBITS, which occupy no file space.
	payload []byte
}

type elfImage struct {
	header   []byte
	sections []rawSection
	shstrndx int
}

func readELFImage(data []byte) (*elfImage, error) {
	if len(data) < elfHeaderSize || !bytes.Equal(data[:4], []byte("\x7fELF")) || data[4] != 2 {
		return nil, fmt.Errorf("not an ELF64 file")
	}
	shoff := binary.LittleEndian.Uint64(data[ehShoff:])
	shentsize := binary.LittleEndian.Uint16(data[ehShentsize:])
	shnum := binary.LittleEndian.Uint16(data[ehShnum:])
	shstrndx := binary.LittleEndian.Uint16(data[ehShstrndx:])
	if shentsize != elf64ShdrSize || shnum == 0 || shoff+uint64(shnum)*elf64ShdrSize > uint64(len(data)) {
		return nil, fmt.Errorf("bad section header table")
	}

	img := &elfImage{
		header:   append([]byte(nil), data[:elfHeaderSize]...),
		sections: make([]rawSection, 0, shnum),
		shstrndx: int(shstrndx),
	}
	nameOffs := make([]uint32, 0, shnum)
	for i := 0; i < int(shnum); i++ {
		base := shoff + uint64(i)*elf64ShdrSize
		sec := rawSection{
			typ:     binary.LittleEndian.Uint32(data[base+shType:]),
			flags:   binary.LittleEndian.Uint64(data[base+shFlags:]),
			addr:    binary.LittleEndian.Uint64(data[base+shAddr:]),
			size:    binary.LittleEndian.Uint64(data[base+shSize:]),
			link:    binary.LittleEndian.Uint32(data[base+shLink:]),
			info:    binary.LittleEndian.Uint32(data[base+shInfo:]),
			align:   binary.LittleEndian.Uint64(data[base+shAddralign:]),
			entsize: binary.LittleEndian.Uint64(data[base+shEntsize:]),
		}
		if sec.typ != uint32(elf.SHT_NULL) && sec.typ != uint32(elf.SHT_NOBITS) && sec.size > 0 {
			off := binary.LittleEndian.Uint64(data[base+shOffset:])
			if off+sec.size > uint64(len(data)) {
				return nil, fmt.Errorf("section %d payload is out of range", i)
			}
			sec.payload = append([]byte(nil), data[off:off+sec.size]...)
		}
		img.sections = append(img.sections, sec)
		nameOffs = append(nameOffs, binary.LittleEndian.Uint32(data[base+shName:]))
	}
	if img.shstrndx <= 0 || img.shstrndx >= len(img.sections) {
		return nil, fmt.Errorf("bad shstrtab index %d", img.shstrndx)
	}
	shstr := img.sections[img.shstrndx].payload
	for i := range img.sections {
		img.sections[i].name = cString(shstr, nameOffs[i])
	}
	return img, nil
}

func (img *elfImage) sectionByName(name string) int {
	for i := range img.sections {
		if img.sections[i].name == name {
			return i
		}
	}
	return -1
}

// rawSymbolIndex returns the symbol table index of name, as used in r_info.
func (img *elfImage) rawSymbolIndex(symtabSec, strtabSec int, name string) (uint32, error) {
	symtab := img.sections[symtabSec].payload
	strtab := img.sections[strtabSec].payload
	for off := 0; off+elf64SymSize <= len(symtab); off += elf64SymSize {
		if cString(strtab, binary.LittleEndian.Uint32(symtab[off+symName:])) == name {
			return uint32(off / elf64SymSize), nil
		}
	}
	return 0, fmt.Errorf("symbol %q not found", name)
}

// symbolValue returns the value of name, requiring it to be defined in
// wantSection.
func (img *elfImage) symbolValue(symtabSec, strtabSec int, name string, wantSection int) (uint64, error) {
	idx, err := img.rawSymbolIndex(symtabSec, strtabSec, name)
	if err != nil {
		return 0, err
	}
	off := int(idx) * elf64SymSize
	symtab := img.sections[symtabSec].payload
	if shndx := int(binary.LittleEndian.Uint16(symtab[off+symShndx:])); shndx != wantSection {
		return 0, fmt.Errorf("symbol %q is in section %d, want %d", name, shndx, wantSection)
	}
	return binary.LittleEndian.Uint64(symtab[off+symValue:]), nil
}

// shiftSectionSymbols moves every symbol defined in section at or after `end`
// down by delta, and rejects any symbol defined inside [start, end) — that range
// is about to disappear, so a definition there would be left dangling.
func (img *elfImage) shiftSectionSymbols(symtabSec, section int, start, end, delta uint64) error {
	symtab := img.sections[symtabSec].payload
	strtab := img.sections[int(img.sections[symtabSec].link)].payload
	for off := 0; off+elf64SymSize <= len(symtab); off += elf64SymSize {
		if int(binary.LittleEndian.Uint16(symtab[off+symShndx:])) != section {
			continue
		}
		value := binary.LittleEndian.Uint64(symtab[off+symValue:])
		switch {
		case value >= end:
			binary.LittleEndian.PutUint64(symtab[off+symValue:], value-delta)
		case value >= start:
			return fmt.Errorf("symbol %q is defined inside the folded range",
				cString(strtab, binary.LittleEndian.Uint32(symtab[off+symName:])))
		}
	}
	return nil
}

// retargetSymbolAddends rewrites every relocation against sym, in every
// relocation section, through remap. remap receives the current addend and
// returns the symbol and addend to store. It returns the number of relocations
// visited, which callers use to check that no reference was missed.
func (img *elfImage) retargetSymbolAddends(sym uint32, remap func(addend uint64) (uint32, uint64)) int {
	visited := 0
	for i := range img.sections {
		if img.sections[i].typ != uint32(elf.SHT_RELA) {
			continue
		}
		rela := img.sections[i].payload
		for off := 0; off+elf64RelaSize <= len(rela); off += elf64RelaSize {
			info := binary.LittleEndian.Uint64(rela[off+8:])
			if uint32(info>>32) != sym {
				continue
			}
			newSym, newAddend := remap(binary.LittleEndian.Uint64(rela[off+16:]))
			binary.LittleEndian.PutUint64(rela[off+8:], uint64(newSym)<<32|info&0xffffffff)
			binary.LittleEndian.PutUint64(rela[off+16:], newAddend)
			visited++
		}
	}
	return visited
}

// appendSection adds sec at the end of the section header table and returns its
// index. Appending keeps every existing section index — and therefore every
// sh_link/sh_info and symbol st_shndx — valid.
func (img *elfImage) appendSection(sec rawSection) int {
	shstr := img.sections[img.shstrndx].payload
	shstr = append(shstr, []byte(sec.name)...)
	shstr = append(shstr, 0)
	img.sections[img.shstrndx].payload = shstr
	img.sections[img.shstrndx].size = uint64(len(shstr))
	img.sections = append(img.sections, sec)
	return len(img.sections) - 1
}

// appendGlobalObjectSymbol adds a global STT_OBJECT symbol and returns its raw
// index. It must be global: symbols are appended at the end of the table, and
// ELF requires every local symbol to precede the first global one (sh_info).
func (img *elfImage) appendGlobalObjectSymbol(symtabSec, strtabSec int, name string, section int, size uint64) (uint32, error) {
	if _, err := img.rawSymbolIndex(symtabSec, strtabSec, name); err == nil {
		return 0, fmt.Errorf("symbol %q already exists", name)
	}
	strtab := img.sections[strtabSec].payload
	nameOff := uint32(len(strtab))
	strtab = append(strtab, []byte(name)...)
	strtab = append(strtab, 0)
	img.sections[strtabSec].payload = strtab
	img.sections[strtabSec].size = uint64(len(strtab))

	var sym [elf64SymSize]byte
	binary.LittleEndian.PutUint32(sym[symName:], nameOff)
	sym[symInfo] = byte(elf.ST_INFO(elf.STB_GLOBAL, elf.STT_OBJECT))
	binary.LittleEndian.PutUint16(sym[symShndx:], uint16(section))
	binary.LittleEndian.PutUint64(sym[symSize:], size)

	symtab := img.sections[symtabSec].payload
	idx := uint32(len(symtab) / elf64SymSize)
	symtab = append(symtab, sym[:]...)
	img.sections[symtabSec].payload = symtab
	img.sections[symtabSec].size = uint64(len(symtab))
	return idx, nil
}

// write lays the image out again: ELF header, section payloads in section
// order, then the section header table.
func (img *elfImage) write() []byte {
	out := make([]byte, elfHeaderSize, elfHeaderSize+1<<20)
	copy(out, img.header)

	offsets := make([]uint64, len(img.sections))
	for i := range img.sections {
		sec := &img.sections[i]
		if sec.payload == nil {
			// SHT_NULL and SHT_NOBITS occupy no file space, but keep a
			// plausible offset so readers that ignore the type stay sane.
			offsets[i] = uint64(len(out))
			continue
		}
		if sec.align > 1 {
			if pad := uint64(len(out)) % sec.align; pad != 0 {
				out = append(out, make([]byte, sec.align-pad)...)
			}
		}
		offsets[i] = uint64(len(out))
		out = append(out, sec.payload...)
	}
	if pad := len(out) % 8; pad != 0 {
		out = append(out, make([]byte, 8-pad)...)
	}

	shoff := uint64(len(out))
	for i := range img.sections {
		sec := &img.sections[i]
		var shdr [elf64ShdrSize]byte
		binary.LittleEndian.PutUint32(shdr[shName:], img.sectionNameOffset(i))
		binary.LittleEndian.PutUint32(shdr[shType:], sec.typ)
		binary.LittleEndian.PutUint64(shdr[shFlags:], sec.flags)
		binary.LittleEndian.PutUint64(shdr[shAddr:], sec.addr)
		binary.LittleEndian.PutUint64(shdr[shOffset:], offsets[i])
		binary.LittleEndian.PutUint64(shdr[shSize:], sec.size)
		binary.LittleEndian.PutUint32(shdr[shLink:], sec.link)
		binary.LittleEndian.PutUint32(shdr[shInfo:], sec.info)
		binary.LittleEndian.PutUint64(shdr[shAddralign:], sec.align)
		binary.LittleEndian.PutUint64(shdr[shEntsize:], sec.entsize)
		out = append(out, shdr[:]...)
	}

	binary.LittleEndian.PutUint64(out[ehShoff:], shoff)
	binary.LittleEndian.PutUint16(out[ehShnum:], uint16(len(img.sections)))
	binary.LittleEndian.PutUint16(out[ehShstrndx:], uint16(img.shstrndx))
	return out
}

// sectionNameOffset finds a section's name in .shstrtab. Names are looked up
// rather than tracked because appendSection may have rewritten the table.
func (img *elfImage) sectionNameOffset(idx int) uint32 {
	shstr := img.sections[img.shstrndx].payload
	name := img.sections[idx].name
	if name == "" {
		return 0
	}
	needle := append([]byte(name), 0)
	if at := bytes.Index(shstr, needle); at >= 0 {
		return uint32(at)
	}
	return 0
}

func cString(data []byte, off uint32) string {
	if int(off) >= len(data) {
		return ""
	}
	end := bytes.IndexByte(data[off:], 0)
	if end < 0 {
		return string(data[off:])
	}
	return string(data[off : int(off)+end])
}
