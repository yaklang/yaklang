//go:build linux

package patch

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	// Go 1.27 amd64 runtime.moduledata.textsectmap. Go 1.26 kept this slice at
	// 0x150; 1.27 inserts typedesclen, itaboffset and itabsize ahead of it.
	// ftab/minpc/text stay at 0x80/0xa0/0xb0. Each textsect is still 24 bytes.
	moduledataTextsectMapOff = 0x168
	moduledataTextsectMapLen = moduledataTextsectMapOff + 8
	moduledataTextsectMapCap = moduledataTextsectMapOff + 16
	moduledataTextsectMapEnd = moduledataTextsectMapOff + 24
	textsectEntrySize        = 24

	moduledataFtabOff  = 0x80
	moduledataFtabLen  = moduledataFtabOff + 8
	moduledataMinpcOff = 0xa0
	moduledataMaxpcOff = 0xa8
	moduledataTextOff  = 0xb0
	moduledataEtextOff = 0xb8
	functabEntrySize   = 8
)

// SortFinalTextMap reorders runtime.textsectmap entries in a linked static
// binary by baseaddr (physical address). The Go runtime pcToOffset iterates
// the table and stops at the first entry whose baseaddr is above the PC, so
// the table must be sorted by baseaddr even though elfsplit emits entries in
// original vaddr order. Sorting happens after lld resolves the section
// addresses, so it does not need to predict lld's final layout.
func SortFinalTextMap(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read final binary: %w", err)
	}
	if len(raw) < 0x40 || string(raw[:4]) != "\x7fELF" {
		return nil
	}
	sections, _, _, _, err := parseELFSections(raw)
	if err != nil {
		return fmt.Errorf("parse final ELF: %w", err)
	}
	// lld can place runtime.firstmoduledata after other bytes in the output
	// .go.module section. Reading the slice header from the section start then
	// lands on ftab: len is the function count and each "entry" is three
	// packed functab uint32 pairs. The symbol is the real moduledata.
	md, found, fromSymbol, err := finalFirstModuleOff(raw, sections)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if md < 0 || md+moduledataTextsectMapEnd > len(raw) {
		return fmt.Errorf("final module data too small")
	}
	mapPtr := binary.LittleEndian.Uint64(raw[md+moduledataTextsectMapOff:])
	mapLen := binary.LittleEndian.Uint64(raw[md+moduledataTextsectMapLen:])
	mapCap := binary.LittleEndian.Uint64(raw[md+moduledataTextsectMapCap:])
	if fromSymbol {
		mapPtr, mapLen, mapCap, err = alignFinalTextsectHeader(raw, sections, md, mapPtr, mapLen, mapCap)
		if err != nil {
			return err
		}
	}
	if mapLen <= 1 {
		return nil
	}
	if mapLen != mapCap {
		return fmt.Errorf("final textsectmap len/cap mismatch: %d/%d", mapLen, mapCap)
	}
	mapOff, err := vmaToFileOffset(raw, sections, mapPtr)
	if err != nil {
		return fmt.Errorf("locate final textsectmap: %w", err)
	}
	if mapOff+int(mapLen)*textsectEntrySize > len(raw) {
		return fmt.Errorf("final textsectmap out of range")
	}

	entries := make([]textsectFinalEntry, int(mapLen))
	var maxOff uint64
	for i := range entries {
		base := mapOff + i*textsectEntrySize
		entries[i] = textsectFinalEntry{
			vaddr:    binary.LittleEndian.Uint64(raw[base:]),
			end:      binary.LittleEndian.Uint64(raw[base+8:]),
			baseaddr: binary.LittleEndian.Uint64(raw[base+16:]),
		}
		if entries[i].end > maxOff {
			maxOff = entries[i].end
		}
	}
	if maxOff > 0xffffffff {
		return fmt.Errorf("%s md=%#x symbol=%v", formatTextMapEndOverflow(raw, sections, mapPtr, entries, maxOff), md, fromSymbol)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].baseaddr < entries[j].baseaddr
	})
	for i := range entries {
		base := mapOff + i*textsectEntrySize
		binary.LittleEndian.PutUint64(raw[base:], entries[i].vaddr)
		binary.LittleEndian.PutUint64(raw[base+8:], entries[i].end)
		binary.LittleEndian.PutUint64(raw[base+16:], entries[i].baseaddr)
	}
	// Sanity check: after sorting, pcToOffset's early exit is valid.
	for i := 1; i < len(entries); i++ {
		if entries[i].baseaddr < entries[i-1].baseaddr {
			return fmt.Errorf("final textsectmap still unsorted at %d", i)
		}
	}
	// findmoduledatap only accepts PCs inside [minpc, maxpc). The packed
	// replacement .text is followed by the .modtext.* sections, so maxpc must
	// be extended to the physical end of all text (runtime.etext), otherwise
	// every module function PC is rejected before findfunc's table lookup.
	text := binary.LittleEndian.Uint64(raw[md+moduledataTextOff:])
	etext := binary.LittleEndian.Uint64(raw[md+moduledataEtextOff:])
	if etext < text || etext-text > 0xffffffff {
		return fmt.Errorf("final etext/text range invalid: text=%#x etext=%#x", text, etext)
	}
	binary.LittleEndian.PutUint64(raw[md+moduledataMinpcOff:], text)
	binary.LittleEndian.PutUint64(raw[md+moduledataMaxpcOff:], etext)
	// moduledataverify recomputes maxpc as textAddr(ftab[last].entryoff) and
	// also requires the ftab to stay sorted by entryoff. elfsplit emits a
	// sentinel textsectmap entry [originalTextSize, +1) -> runtime.etext, so
	// re-point the ftab sentinel at the original text end: the table stays
	// sorted (its offset is the largest) and textAddr resolves it to etext.
	ftabPtr := binary.LittleEndian.Uint64(raw[md+moduledataFtabOff:])
	ftabLen := binary.LittleEndian.Uint64(raw[md+moduledataFtabLen:])
	if ftabLen == 0 {
		return fmt.Errorf("final ftab empty")
	}
	ftabOff, err := vmaToFileOffset(raw, sections, ftabPtr)
	if err != nil {
		return fmt.Errorf("locate final ftab: %w", err)
	}
	sentinel := int(ftabLen-1) * functabEntrySize
	if ftabOff+sentinel+functabEntrySize > len(raw) {
		return fmt.Errorf("final ftab out of range")
	}
	if maxOff > 0xffffffff {
		return fmt.Errorf("final original text end %#x exceeds uint32", maxOff)
	}
	var sentinelFound bool
	for _, e := range entries {
		if e.vaddr == maxOff-1 && e.end == maxOff && e.baseaddr == etext {
			sentinelFound = true
			break
		}
	}
	if !sentinelFound {
		return fmt.Errorf("final textsectmap missing etext sentinel entry at %#x", maxOff)
	}
	binary.LittleEndian.PutUint32(raw[ftabOff+sentinel:], uint32(maxOff-1))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write sorted textsectmap: %w", err)
	}
	return nil
}

type textsectFinalEntry struct {
	vaddr    uint64
	end      uint64
	baseaddr uint64
}

// formatTextMapEndOverflow describes the entry whose original-text end does not
// fit in a functab entryoff. The object file's ends are all small; a value
// this large is a relocated address read as end, or a read that started at
// the wrong place. The section and the neighboring entries distinguish those.
func formatTextMapEndOverflow(raw []byte, sections []elfSection, mapPtr uint64, entries []textsectFinalEntry, maxOff uint64) string {
	secName := "?"
	var secAddr, secOff, secSize uint64
	for i := range sections {
		s := &sections[i]
		if s.typ != uint32(1) || s.size == 0 || s.flags&2 == 0 {
			continue
		}
		if mapPtr >= s.addr && mapPtr < s.addr+s.size {
			secName = s.name
			secAddr, secOff, secSize = s.addr, s.offset, s.size
			break
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "final original text end %#x exceeds uint32 (len=%d mapPtr=%#x section=%s addr=%#x off=%#x size=%#x file=%d)",
		maxOff, len(entries), mapPtr, secName, secAddr, secOff, secSize, len(raw))
	shown := 0
	bad := 0
	for i := range entries {
		if entries[i].end <= 0xffffffff {
			continue
		}
		bad++
		if shown >= 6 && entries[i].end != maxOff {
			continue
		}
		fmt.Fprintf(&b, " [%d v=%#x e=%#x b=%#x]", i, entries[i].vaddr, entries[i].end, entries[i].baseaddr)
		shown++
	}
	fmt.Fprintf(&b, " bad=%d", bad)
	if n := len(entries); n > 0 {
		e0, el := entries[0], entries[n-1]
		fmt.Fprintf(&b, " first=[v=%#x e=%#x b=%#x] last=[v=%#x e=%#x b=%#x]",
			e0.vaddr, e0.end, e0.baseaddr, el.vaddr, el.end, el.baseaddr)
	}
	return b.String()
}

func vmaToFileOffset(raw []byte, sections []elfSection, vma uint64) (int, error) {
	for i := range sections {
		s := &sections[i]
		if s.typ != uint32(1) { // SHT_PROGBITS
			continue
		}
		if s.size == 0 || s.flags&2 == 0 { // SHF_ALLOC
			continue
		}
		if vma >= s.addr && vma < s.addr+s.size {
			fileOff := s.offset + (vma - s.addr)
			if fileOff >= uint64(len(raw)) {
				return 0, fmt.Errorf("section %s out of range", s.name)
			}
			return int(fileOff), nil
		}
	}
	return 0, fmt.Errorf("VMA %#x not in any allocated section", vma)
}

type elfNamedSym struct {
	name  string
	shndx uint16
	value uint64
	size  uint64
	bind  uint8
}

// finalFirstModuleOff returns the file offset of runtime.firstmoduledata.
// found is false when the binary has neither the symbol nor a .go.module
// section. fromSymbol is true when the offset came from the symbol rather
// than the start of .go.module.
func finalFirstModuleOff(raw []byte, sections []elfSection) (int, bool, bool, error) {
	syms, err := elfSymbolsNamed(raw, sections, "runtime.firstmoduledata")
	if err != nil {
		return 0, false, false, err
	}
	if sym, ok := preferredSymbol(syms, len(sections)); ok {
		off, err := symbolFileOffset(raw, sections, sym)
		if err != nil {
			return 0, false, false, err
		}
		return off, true, true, nil
	}
	for i := range sections {
		if sections[i].name == ".go.module" {
			return int(sections[i].offset), true, false, nil
		}
	}
	return 0, false, false, nil
}

// alignFinalTextsectHeader points the moduledata slice at runtime.textsectionmap
// when the header length disagrees with that symbol. elfsplit writes one
// 24-byte entry per function; a header read from the wrong place reports the
// functab length instead. The symbol size is the table the runtime must use.
func alignFinalTextsectHeader(raw []byte, sections []elfSection, md int, mapPtr, mapLen, mapCap uint64) (uint64, uint64, uint64, error) {
	eType := binary.LittleEndian.Uint16(raw[16:])
	if eType != uint16(elf.ET_EXEC) && eType != uint16(elf.ET_DYN) {
		return mapPtr, mapLen, mapCap, nil
	}
	syms, err := elfSymbolsNamed(raw, sections, "runtime.textsectionmap")
	if err != nil || len(syms) == 0 {
		return mapPtr, mapLen, mapCap, err
	}
	sym, ok := preferredSymbol(syms, len(sections))
	if !ok || sym.size == 0 || sym.size%textsectEntrySize != 0 {
		return mapPtr, mapLen, mapCap, nil
	}
	wantLen := sym.size / uint64(textsectEntrySize)
	wantPtr := sym.value
	if mapPtr == wantPtr && mapLen == wantLen && mapCap == wantLen {
		return mapPtr, mapLen, mapCap, nil
	}
	if md < 0 || md+moduledataTextsectMapEnd > len(raw) {
		return 0, 0, 0, fmt.Errorf("module data too small to retarget textsectmap")
	}
	binary.LittleEndian.PutUint64(raw[md+moduledataTextsectMapOff:], wantPtr)
	binary.LittleEndian.PutUint64(raw[md+moduledataTextsectMapLen:], wantLen)
	binary.LittleEndian.PutUint64(raw[md+moduledataTextsectMapCap:], wantLen)
	return wantPtr, wantLen, wantLen, nil
}

func elfSymbolsNamed(raw []byte, sections []elfSection, want string) ([]elfNamedSym, error) {
	var symtab *elfSection
	for i := range sections {
		if sections[i].name == ".symtab" && sections[i].typ == uint32(elf.SHT_SYMTAB) {
			symtab = &sections[i]
			break
		}
	}
	if symtab == nil || symtab.size == 0 {
		return nil, nil
	}
	if int(symtab.link) >= len(sections) {
		return nil, fmt.Errorf("symtab link out of range")
	}
	strtab := sections[symtab.link]
	if int(symtab.offset)+int(symtab.size) > len(raw) || int(strtab.offset)+int(strtab.size) > len(raw) {
		return nil, fmt.Errorf("symtab/strtab out of range")
	}
	strs := raw[strtab.offset : strtab.offset+strtab.size]
	count := int(symtab.size) / elf64SymSize
	var out []elfNamedSym
	for i := 0; i < count; i++ {
		off := int(symtab.offset) + i*elf64SymSize
		stName := binary.LittleEndian.Uint32(raw[off:])
		if stName == 0 || int(stName) >= len(strs) {
			continue
		}
		end := bytes.IndexByte(strs[stName:], 0)
		if end <= 0 {
			continue
		}
		name := string(strs[stName : int(stName)+end])
		if name != want {
			continue
		}
		out = append(out, elfNamedSym{
			name:  name,
			shndx: binary.LittleEndian.Uint16(raw[off+6:]),
			value: binary.LittleEndian.Uint64(raw[off+8:]),
			size:  binary.LittleEndian.Uint64(raw[off+16:]),
			bind:  raw[off+4] >> 4,
		})
	}
	return out, nil
}

func symbolDefined(sym elfNamedSym, nsec int) bool {
	if sym.shndx == 0 || sym.shndx >= 0xff00 {
		return false
	}
	return int(sym.shndx) < nsec
}

func preferredSymbol(syms []elfNamedSym, nsec int) (elfNamedSym, bool) {
	var best elfNamedSym
	found := false
	for _, sym := range syms {
		if !symbolDefined(sym, nsec) {
			continue
		}
		if !found || sym.size > best.size || (sym.size == best.size && sym.bind == uint8(elf.STB_GLOBAL) && best.bind != uint8(elf.STB_GLOBAL)) {
			best = sym
			found = true
		}
	}
	return best, found
}

func symbolFileOffset(raw []byte, sections []elfSection, sym elfNamedSym) (int, error) {
	if !symbolDefined(sym, len(sections)) {
		return 0, fmt.Errorf("symbol %s has no section", sym.name)
	}
	sec := sections[sym.shndx]
	eType := binary.LittleEndian.Uint16(raw[16:])
	var delta uint64
	switch elf.Type(eType) {
	case elf.ET_REL:
		delta = sym.value
	default:
		if sym.value < sec.addr {
			return 0, fmt.Errorf("symbol %s value %#x before section %s %#x", sym.name, sym.value, sec.name, sec.addr)
		}
		delta = sym.value - sec.addr
	}
	if sec.size > 0 && delta >= sec.size {
		return 0, fmt.Errorf("symbol %s offset %#x outside section %s", sym.name, delta, sec.name)
	}
	off := sec.offset + delta
	if off >= uint64(len(raw)) {
		return 0, fmt.Errorf("symbol %s file offset out of range", sym.name)
	}
	return int(off), nil
}

// MissingRetainedModules reports yaklib modules whose .modtext section was
// retained by the linker but has no valid textsectmap entry. patch clears the
// textmap relocations of modules it treats as unused; if the base runtime's
// init graph still references such a module, lld keeps the section while the
// runtime loses PC lookup for it (findfunc/traceback break). The compiler
// re-links with these modules marked used.
func MissingRetainedModules(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read final binary: %w", err)
	}
	if len(raw) < 0x40 || string(raw[:4]) != "\x7fELF" {
		return nil, nil
	}
	sections, _, _, _, err := parseELFSections(raw)
	if err != nil {
		return nil, fmt.Errorf("parse final ELF: %w", err)
	}
	type modSection struct {
		name string
		addr uint64
		end  uint64
	}
	var retained []modSection
	for i := range sections {
		s := &sections[i]
		if !strings.HasPrefix(s.name, ".modtext.") || s.flags&2 == 0 || s.size == 0 {
			continue
		}
		retained = append(retained, modSection{
			name: strings.TrimPrefix(s.name, ".modtext."),
			addr: s.addr,
			end:  s.addr + s.size,
		})
	}
	if len(retained) == 0 {
		return nil, nil
	}
	md, found, _, err := finalFirstModuleOff(raw, sections)
	if err != nil {
		return nil, err
	}
	if !found || md < 0 || md+moduledataTextsectMapEnd > len(raw) {
		return nil, fmt.Errorf("final .go.module missing or too small")
	}
	mapPtr := binary.LittleEndian.Uint64(raw[md+moduledataTextsectMapOff:])
	mapLen := binary.LittleEndian.Uint64(raw[md+moduledataTextsectMapLen:])
	mapCap := binary.LittleEndian.Uint64(raw[md+moduledataTextsectMapCap:])
	if mapLen == 0 || mapLen != mapCap {
		return nil, nil
	}
	mapOff, err := vmaToFileOffset(raw, sections, mapPtr)
	if err != nil {
		return nil, fmt.Errorf("locate final textsectmap: %w", err)
	}
	if mapOff+int(mapLen)*textsectEntrySize > len(raw) {
		return nil, fmt.Errorf("final textsectmap out of range")
	}
	covered := make(map[string]bool, len(retained))
	for i := uint64(0); i < mapLen; i++ {
		base := mapOff + int(i)*textsectEntrySize
		vaddr := binary.LittleEndian.Uint64(raw[base:])
		end := binary.LittleEndian.Uint64(raw[base+8:])
		baseaddr := binary.LittleEndian.Uint64(raw[base+16:])
		if baseaddr == 0 {
			continue
		}
		physEnd := baseaddr + (end - vaddr)
		for _, m := range retained {
			if baseaddr < m.end && m.addr < physEnd {
				covered[m.name] = true
				break
			}
		}
	}
	var missing []string
	for _, m := range retained {
		if !covered[m.name] {
			missing = append(missing, m.name)
		}
	}
	sort.Strings(missing)
	return missing, nil
}
