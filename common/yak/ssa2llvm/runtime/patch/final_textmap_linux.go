//go:build linux

package patch

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	moduledataTextsectMapOff = 0x150
	moduledataTextsectMapLen = moduledataTextsectMapOff + 8
	moduledataTextsectMapCap = moduledataTextsectMapOff + 16
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
	var goModule *elfSection
	for i := range sections {
		if sections[i].name == ".go.module" {
			goModule = &sections[i]
			break
		}
	}
	if goModule == nil {
		// Not a Go runtime binary (or no module data); nothing to do.
		return nil
	}
	if int(goModule.offset)+0x168 > len(raw) {
		return fmt.Errorf("final .go.module too small")
	}
	md := int(goModule.offset)
	mapPtr := binary.LittleEndian.Uint64(raw[md+moduledataTextsectMapOff:])
	mapLen := binary.LittleEndian.Uint64(raw[md+moduledataTextsectMapLen:])
	mapCap := binary.LittleEndian.Uint64(raw[md+moduledataTextsectMapCap:])
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

	type entry struct {
		vaddr    uint64
		end      uint64
		baseaddr uint64
	}
	entries := make([]entry, int(mapLen))
	var maxOff uint64
	for i := range entries {
		base := mapOff + i*textsectEntrySize
		entries[i] = entry{
			vaddr:    binary.LittleEndian.Uint64(raw[base:]),
			end:      binary.LittleEndian.Uint64(raw[base+8:]),
			baseaddr: binary.LittleEndian.Uint64(raw[base+16:]),
		}
		if entries[i].end > maxOff {
			maxOff = entries[i].end
		}
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
	var goModule *elfSection
	for i := range sections {
		if sections[i].name == ".go.module" {
			goModule = &sections[i]
			break
		}
	}
	if goModule == nil || int(goModule.offset)+0x168 > len(raw) {
		return nil, fmt.Errorf("final .go.module missing or too small")
	}
	md := int(goModule.offset)
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
