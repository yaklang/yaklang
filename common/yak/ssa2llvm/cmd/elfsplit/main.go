package main

// elfsplit: Post-process go.o (from Go c-archive) to split .text into
// per-module sections so that lld --gc-sections can drop unused module
// code at ssa2llvm compile time.
//
// Usage: elfsplit <input.go.o> <output.go.o> <comma-separated-module-names>

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/arch/x86/x86asm"
)

const (
	elf64SymSize                = 24
	elf64RelaSize               = 24
	elf64ShdrSize               = 64
	moduledataTextsectMapOffset = 0x150
	moduledataTextsectMapLenOff = moduledataTextsectMapOffset + 8
	moduledataTextsectMapCapOff = moduledataTextsectMapOffset + 16
	textSectionMapEntrySize     = 24
	textSectionMapSectionName   = ".data.rel.ro.yaktextmap"
	textSectionMapRelocName     = ".rela.data.rel.ro.yaktextmap"
	textSectionEndName          = ".zz_yak_text_end"
)

type sectionMeta struct {
	name    string
	idx     int
	typ     uint32
	flags   uint64
	addr    uint64
	offset  uint64
	size    uint64
	link    uint32
	info    uint32
	align   uint64
	entsize uint64
}

type symMeta struct {
	idx   int
	name  string
	info  byte
	other byte
	shndx uint16
	value uint64
	size  uint64
}

type modFunc struct {
	symIdx int
	name   string
	oldOff uint64
	size   uint64
	newOff uint64
	module string
}

// codePlacement describes where a range from the original Go linker's .text
// section is placed in the rewritten object. module=="" means the packed
// replacement .text section; otherwise it is the corresponding .modtext.*
// section.
type codePlacement struct {
	symIdx int
	name   string
	oldOff uint64
	size   uint64
	newOff uint64
	module string
}

type keepChunk struct {
	oldOff uint64
	size   uint64
	newOff uint64
}

// textMapEntry is one runtime.textsectmap record. The base address is emitted
// as an ELF absolute relocation against symIdx plus addend. Keeping one record
// per packed chunk/function avoids overlapping virtual ranges when functions
// from several modules were interleaved by the Go linker.
type textMapEntry struct {
	vaddr  uint64
	end    uint64
	symIdx int
	addend int64
}

type newSec struct {
	name    string
	typ     uint32
	flags   uint64
	data    []byte
	align   uint64
	entsize uint64
	link    uint32
	info    uint32
}

type addedSec struct {
	sec    newSec
	offset uint64
	newIdx int
}

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: elfsplit <input.go.o> <output.go.o> <comma-separated-modules>")
		os.Exit(1)
	}
	inputFile := os.Args[1]
	outputFile := os.Args[2]
	modulesArg := os.Args[3]
	modules := strings.Split(modulesArg, ",")
	for i := range modules {
		modules[i] = strings.TrimSpace(modules[i])
	}

	data, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read: %v\n", err)
		os.Exit(1)
	}

	f, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// Parse section headers
	var secMetaList []sectionMeta
	for i, s := range f.Sections {
		secMetaList = append(secMetaList, sectionMeta{
			name: s.Name, idx: i, typ: uint32(s.Type), flags: uint64(s.Flags),
			addr: s.Addr, offset: s.Offset, size: s.Size, link: s.Link, info: s.Info,
			align: s.Addralign, entsize: s.Entsize,
		})
	}

	textIdx := -1
	for i, s := range secMetaList {
		if s.name == ".text" {
			textIdx = i
			break
		}
	}
	if textIdx < 0 {
		fmt.Fprintln(os.Stderr, "no .text section")
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "elfsplit: .text size=%d (%.1f KB)\n",
		secMetaList[textIdx].size, float64(secMetaList[textIdx].size)/1024)

	// Parse symbols
	elfSyms, err := f.Symbols()
	if err != nil {
		fmt.Fprintf(os.Stderr, "read symbols: %v\n", err)
		os.Exit(1)
	}
	var syms []symMeta
	for i, s := range elfSyms {
		syms = append(syms, symMeta{
			idx: i, name: s.Name, info: byte(s.Info), other: byte(s.Other),
			shndx: uint16(s.Section), value: s.Value, size: s.Size,
		})
	}

	// Find symtab and .rela.text
	symtabIdx := -1
	relaTextIdx := -1
	for i, s := range secMetaList {
		if s.name == ".symtab" {
			symtabIdx = i
		}
		if s.name == ".rela.text" {
			relaTextIdx = i
		}
	}
	if symtabIdx < 0 {
		fmt.Fprintln(os.Stderr, "no .symtab")
		os.Exit(1)
	}

	// Build module -> package path mapping
	modulePkgs := buildModulePackageMap(modules)

	// Collect module function ranges
	var modRanges []modFunc
	modFuncSize := make(map[string]uint64)
	modFuncCount := make(map[string]int)
	for _, s := range syms {
		if int(s.shndx) != textIdx {
			continue
		}
		if elf.SymType(s.info&0x0f) != elf.STT_FUNC {
			continue
		}
		pkg := classifyPackage(s.name)
		mod := matchModule(pkg, modulePkgs)
		if mod == "" {
			mod = matchRegisterSymbol(s.name, modules)
			if mod == "" {
				continue
			}
		}
		modRanges = append(modRanges, modFunc{
			symIdx: s.idx, name: s.name, oldOff: s.value, size: s.size, module: mod,
		})
		modFuncSize[mod] += s.size
		modFuncCount[mod]++
	}

	fmt.Fprintf(os.Stderr, "elfsplit: %d module functions, %d modules\n", len(modRanges), len(modFuncCount))
	for _, mod := range modules {
		if modFuncCount[mod] > 0 {
			fmt.Fprintf(os.Stderr, "  .modtext.%s: %d funcs, %d bytes (%.1f KB)\n",
				mod, modFuncCount[mod], modFuncSize[mod], float64(modFuncSize[mod])/1024)
		}
	}

	if len(modRanges) == 0 {
		if err := os.WriteFile(outputFile, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "elfsplit: no module functions, copied as-is\n")
		return
	}

	// Group by module and sort by old offset
	modGroups := make(map[string][]modFunc)
	for _, r := range modRanges {
		modGroups[r.module] = append(modGroups[r.module], r)
	}
	for mod := range modGroups {
		sort.Slice(modGroups[mod], func(i, j int) bool {
			return modGroups[mod][i].oldOff < modGroups[mod][j].oldOff
		})
	}

	var textFuncs []symMeta
	for _, s := range syms {
		if int(s.shndx) == textIdx && elf.SymType(s.info&0x0f) == elf.STT_FUNC && s.size > 0 {
			textFuncs = append(textFuncs, s)
		}
	}
	sort.Slice(textFuncs, func(i, j int) bool {
		return textFuncs[i].value < textFuncs[j].value
	})

	textSym, hasTextSym := findSymbol(syms, "runtime.text")
	etextSym, hasEtextSym := findSymbol(syms, "runtime.etext")
	textMapSym, hasTextMapSym := findSymbol(syms, "runtime.textsectionmap")
	firstModuleSym, hasFirstModuleSym := findSymbol(syms, "runtime.firstmoduledata")
	if !hasTextSym || !hasEtextSym || !hasTextMapSym || !hasFirstModuleSym {
		fmt.Fprintln(os.Stderr, "elfsplit: missing Go runtime text metadata symbols")
		os.Exit(1)
	}

	originalTextSize := secMetaList[textIdx].size

	// Keep the module sections independently packed. The original .text is
	// replaced by the non-module chunks below, so the bytes of removed module
	// functions do not remain as zero-filled holes in the final executable.
	for mod := range modGroups {
		var off uint64
		for i := range modGroups[mod] {
			off = alignUp(off, 16)
			modGroups[mod][i].newOff = off
			off += modGroups[mod][i].size
		}
	}

	// Go's linker resolves some direct PC-relative branches inside go.o and
	// leaves no ELF relocation for them. Once code is packed into replacement
	// sections, those encoded displacements must be rewritten (or represented
	// by a new relocation when the target remains in another section).
	// movedRanges must be built after module newOff assignment: placements,
	// symbol values, and branch rewriting all use the per-function packed
	// offsets, not the zero values of freshly grouped ranges.
	var movedRanges []modFunc
	for _, ranges := range modGroups {
		movedRanges = append(movedRanges, ranges...)
	}
	sort.Slice(movedRanges, func(i, j int) bool {
		return movedRanges[i].oldOff < movedRanges[j].oldOff
	})

	keepChunks := buildKeepChunks(originalTextSize, movedRanges)
	var packedTextSize uint64
	for i := range keepChunks {
		packedTextSize = alignUp(packedTextSize, 16)
		keepChunks[i].newOff = packedTextSize
		packedTextSize += keepChunks[i].size
	}

	// Build placements for every original function. The Go pclntab keeps the
	// original virtual offsets; runtime.textsectionmap below translates those
	// offsets to the packed physical sections at runtime.
	var placements []codePlacement
	for _, s := range textFuncs {
		if moved := findMovedRange(movedRanges, s.value); moved != nil {
			placements = append(placements, codePlacement{
				symIdx: s.idx,
				name:   s.name,
				oldOff: s.value,
				size:   s.size,
				newOff: moved.newOff + (s.value - moved.oldOff),
				module: moved.module,
			})
			continue
		}
		chunk := findKeepChunk(keepChunks, s.value)
		if chunk == nil || s.value+s.size > chunk.oldOff+chunk.size {
			fmt.Fprintf(os.Stderr, "elfsplit: function %q is not in a retained text chunk\n", s.name)
			os.Exit(1)
		}
		placements = append(placements, codePlacement{
			symIdx: s.idx,
			name:   s.name,
			oldOff: s.value,
			size:   s.size,
			newOff: chunk.newOff + (s.value - chunk.oldOff),
		})
	}
	sort.Slice(placements, func(i, j int) bool {
		if placements[i].oldOff == placements[j].oldOff {
			return placements[i].size < placements[j].size
		}
		return placements[i].oldOff < placements[j].oldOff
	})

	textOff := secMetaList[textIdx].offset
	packedText := make([]byte, packedTextSize)
	for _, chunk := range keepChunks {
		copy(packedText[chunk.newOff:chunk.newOff+chunk.size],
			data[textOff+chunk.oldOff:textOff+chunk.oldOff+chunk.size])
	}

	modDataByModule := make(map[string][]byte)
	for _, mod := range modules {
		ranges := modGroups[mod]
		if len(ranges) == 0 {
			continue
		}
		var totalSize uint64
		for _, r := range ranges {
			totalSize = maxUint64(totalSize, r.newOff+r.size)
		}
		modData := make([]byte, totalSize)
		for _, r := range ranges {
			copy(modData[r.newOff:r.newOff+r.size],
				data[textOff+r.oldOff:textOff+r.oldOff+r.size])
		}
		modDataByModule[mod] = modData
	}

	// A relocation's source offset is in either a retained .text chunk or a
	// moved module function. Keep these ranges separate from function
	// placements because relocations can also land in padding bytes.
	var sourceRanges []codePlacement
	for _, chunk := range keepChunks {
		sourceRanges = append(sourceRanges, codePlacement{
			oldOff: chunk.oldOff,
			size:   chunk.size,
			newOff: chunk.newOff,
		})
	}
	for _, ranges := range modGroups {
		for _, r := range ranges {
			sourceRanges = append(sourceRanges, codePlacement{
				oldOff: r.oldOff,
				size:   r.size,
				newOff: r.newOff,
				module: r.module,
			})
		}
	}
	sort.Slice(sourceRanges, func(i, j int) bool { return sourceRanges[i].oldOff < sourceRanges[j].oldOff })

	textRelocOffsets := make(map[uint64]struct{})
	relocsBySection := map[string][]byte{".text": nil}
	if relaTextIdx >= 0 {
		relaSec := secMetaList[relaTextIdx]
		relaData := data[relaSec.offset : relaSec.offset+relaSec.size]
		for base := uint64(0); base+elf64RelaSize <= uint64(len(relaData)); base += elf64RelaSize {
			rOffset := binary.LittleEndian.Uint64(relaData[base:])
			textRelocOffsets[rOffset] = struct{}{}
			source := findPlacement(sourceRanges, rOffset)
			if source == nil {
				fmt.Fprintf(os.Stderr, "elfsplit: text relocation at %#x is outside retained code\n", rOffset)
				os.Exit(1)
			}
			entry := make([]byte, elf64RelaSize)
			binary.LittleEndian.PutUint64(entry[0:], source.newOff+(rOffset-source.oldOff))
			copy(entry[8:], relaData[base+8:base+16])
			copy(entry[16:], relaData[base+16:base+24])
			section := ".text"
			if source.module != "" {
				section = ".modtext." + source.module
			}
			relocsBySection[section] = append(relocsBySection[section], entry...)
		}
	}

	// Rewrite direct branches after all destinations are known. A branch whose
	// target remains in the same input section is fixed in-place; a cross-section
	// branch receives a synthetic ELF relocation for lld.
	for _, fn := range placements {
		var destination []byte
		section := ".text"
		if fn.module != "" {
			section = ".modtext." + fn.module
			destination = modDataByModule[fn.module]
		} else {
			destination = packedText
		}
		extraRelas, err := rewritePCRelativeBranches(
			data, destination, textOff, fn, placements, textRelocOffsets, originalTextSize,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "elfsplit: rewrite %s/%s branches: %v\n", section, fn.name, err)
			os.Exit(1)
		}
		relocsBySection[section] = append(relocsBySection[section], extraRelas...)
	}
	var newSections []newSec

	// Build output: start from a copy of input. Old section payloads are left
	// unreachable in the object file; their section headers are redirected to
	// the packed payloads appended below. This avoids shifting the large symbol
	// and relocation tables in place.
	out := make([]byte, len(data))
	copy(out, data)

	// ELF header fields
	e_shoff := binary.LittleEndian.Uint64(out[0x28:])
	e_shentsize := binary.LittleEndian.Uint16(out[0x3a:])
	e_shnum := binary.LittleEndian.Uint16(out[0x3c:])
	e_shstrndx := binary.LittleEndian.Uint16(out[0x3e:])

	// Append the packed replacement for .text and repoint its existing section
	// header. The relocation section is handled the same way below.
	curOff := uint64(len(out))
	curOff = alignFileOffset(&out, curOff, 64)
	packedTextFileOff := curOff
	out = append(out, packedText...)
	curOff += uint64(len(packedText))
	textHdrOff := e_shoff + uint64(textIdx)*uint64(e_shentsize)
	binary.LittleEndian.PutUint64(out[textHdrOff+24:], packedTextFileOff)
	binary.LittleEndian.PutUint64(out[textHdrOff+32:], uint64(len(packedText)))

	if relaTextIdx >= 0 {
		relaHdrOff := e_shoff + uint64(relaTextIdx)*uint64(e_shentsize)
		textRelas := relocsBySection[".text"]
		if len(textRelas) == 0 {
			binary.LittleEndian.PutUint64(out[relaHdrOff+24:], 0)
			binary.LittleEndian.PutUint64(out[relaHdrOff+32:], 0)
		} else {
			curOff = alignFileOffset(&out, curOff, 8)
			relaFileOff := curOff
			out = append(out, textRelas...)
			curOff += uint64(len(textRelas))
			binary.LittleEndian.PutUint64(out[relaHdrOff+24:], relaFileOff)
			binary.LittleEndian.PutUint64(out[relaHdrOff+32:], uint64(len(textRelas)))
		}
	} else if len(relocsBySection[".text"]) > 0 {
		newSections = append(newSections, newSec{
			name: ".rela.text", typ: uint32(elf.SHT_RELA), data: relocsBySection[".text"],
			align: 8, entsize: elf64RelaSize, link: uint32(symtabIdx), info: uint32(textIdx),
		})
	}

	// Add module sections, a retained executable end marker for runtime.etext,
	// and the runtime text-section map. The map's relocations are patched out
	// for unused modules by runtime/patch, so it does not make every module a GC
	// root.
	for _, mod := range modules {
		modData, ok := modDataByModule[mod]
		if !ok {
			continue
		}
		newSections = append(newSections, newSec{
			name: ".modtext." + mod, typ: uint32(elf.SHT_PROGBITS),
			flags: uint64(elf.SHF_ALLOC | elf.SHF_EXECINSTR), data: modData, align: 64,
		})
		if relas := relocsBySection[".modtext."+mod]; len(relas) > 0 {
			newSections = append(newSections, newSec{
				name: ".rela.modtext." + mod, typ: uint32(elf.SHT_RELA), data: relas,
				align: 8, entsize: elf64RelaSize, link: uint32(symtabIdx),
			})
		}
	}
	textEndData := []byte{0xc3}
	newSections = append(newSections, newSec{
		name: textSectionEndName, typ: uint32(elf.SHT_PROGBITS),
		flags: uint64(elf.SHF_ALLOC | elf.SHF_EXECINSTR), data: textEndData, align: 16,
	})

	textMapEntries := buildTextMapEntries(keepChunks, modGroups, modules, textSym.idx, originalTextSize, etextSym.idx)
	textMapData, textMapRelas := encodeTextMap(textMapEntries)
	newSections = append(newSections, newSec{
		name: textSectionMapSectionName, typ: uint32(elf.SHT_PROGBITS),
		flags: uint64(elf.SHF_ALLOC | elf.SHF_WRITE), data: textMapData, align: 8,
	})
	newSections = append(newSections, newSec{
		name: textSectionMapRelocName, typ: uint32(elf.SHT_RELA), data: textMapRelas,
		align: 8, entsize: elf64RelaSize, link: uint32(symtabIdx),
	})

	newSecStartIdx := int(e_shnum)
	var added []addedSec
	for i, ns := range newSections {
		if ns.align > 1 {
			curOff = alignFileOffset(&out, curOff, ns.align)
		}
		added = append(added, addedSec{sec: ns, offset: curOff, newIdx: newSecStartIdx + i})
		out = append(out, ns.data...)
		curOff += uint64(len(ns.data))
	}

	// Set each new RELA section's sh_info to its target section index.
	for i := range added {
		if added[i].sec.typ != uint32(elf.SHT_RELA) {
			continue
		}
		targetName := "." + strings.TrimPrefix(added[i].sec.name, ".rela.")
		if targetName == ".text" {
			added[i].sec.info = uint32(textIdx)
			continue
		}
		for j := range added {
			if added[j].sec.name == targetName {
				added[i].sec.info = uint32(added[j].newIdx)
				break
			}
		}
	}

	// Update the symbol table for every function and metadata symbol whose
	// section/value changed.
	symtabOff := secMetaList[symtabIdx].offset
	for _, p := range placements {
		sectionIdx := textIdx
		if p.module != "" {
			sectionIdx = findAddedSection(added, ".modtext."+p.module)
			if sectionIdx < 0 {
				fmt.Fprintf(os.Stderr, "elfsplit: missing output section for module %s\n", p.module)
				os.Exit(1)
			}
		}
		writeSymbolPlacement(out, symtabOff, p.symIdx, sectionIdx, p.newOff)
	}

	textMapIdx := findAddedSection(added, textSectionMapSectionName)
	textEndIdx := findAddedSection(added, textSectionEndName)
	if textMapIdx < 0 || textEndIdx < 0 {
		fmt.Fprintln(os.Stderr, "elfsplit: missing generated runtime metadata section")
		os.Exit(1)
	}
	writeSymbolPlacement(out, symtabOff, textMapSym.idx, textMapIdx, 0)
	writeSymbolSize(out, symtabOff, textMapSym.idx, uint64(len(textMapData)))
	writeSymbolPlacement(out, symtabOff, etextSym.idx, textEndIdx, uint64(len(textEndData)))

	firstModuleSecIdx := int(firstModuleSym.shndx)
	if firstModuleSecIdx < 0 || firstModuleSecIdx >= len(secMetaList) {
		fmt.Fprintln(os.Stderr, "elfsplit: runtime.firstmoduledata has invalid section")
		os.Exit(1)
	}
	firstModuleSec := secMetaList[firstModuleSecIdx]
	firstModuleOff := firstModuleSec.offset + firstModuleSym.value
	if firstModuleOff+moduledataTextsectMapCapOff+8 > uint64(len(out)) {
		fmt.Fprintln(os.Stderr, "elfsplit: runtime.firstmoduledata is too small")
		os.Exit(1)
	}
	binary.LittleEndian.PutUint64(out[firstModuleOff+moduledataTextsectMapLenOff:], uint64(len(textMapEntries)))
	binary.LittleEndian.PutUint64(out[firstModuleOff+moduledataTextsectMapCapOff:], uint64(len(textMapEntries)))

	// 5. Append new section names to .shstrtab
	shstrtabSec := secMetaList[e_shstrndx]
	oldShstrtab := out[shstrtabSec.offset : shstrtabSec.offset+shstrtabSec.size]
	newShstrtab := make([]byte, len(oldShstrtab))
	copy(newShstrtab, oldShstrtab)
	type nameEntry struct{ off uint32 }
	names := make([]nameEntry, len(added))
	for i, a := range added {
		off := uint32(len(newShstrtab))
		newShstrtab = append(newShstrtab, []byte(a.sec.name)...)
		newShstrtab = append(newShstrtab, 0)
		names[i] = nameEntry{off: off}
	}
	// Append the new part of shstrtab to the file
	if curOff%1 != 0 {
		// no alignment needed
	}
	out = append(out, newShstrtab...)
	curOff = uint64(len(out))

	// Update .shstrtab section header: offset and size
	shstrtabHdrOff := e_shoff + uint64(e_shstrndx)*uint64(e_shentsize)
	newShstrtabFileOff := uint64(len(out)) - uint64(len(newShstrtab))
	binary.LittleEndian.PutUint64(out[shstrtabHdrOff+24:], newShstrtabFileOff)       // sh_offset
	binary.LittleEndian.PutUint64(out[shstrtabHdrOff+32:], uint64(len(newShstrtab))) // sh_size

	// 6. Build new section header table: copy old + append new
	if curOff%8 != 0 {
		pad := 8 - (curOff % 8)
		out = append(out, make([]byte, pad)...)
		curOff += pad
	}
	allShdrOff := curOff
	// Copy old section headers
	oldShdrs := out[e_shoff : e_shoff+uint64(e_shnum)*uint64(e_shentsize)]
	out = append(out, oldShdrs...)
	// Append new section headers
	for i, a := range added {
		var shdr [elf64ShdrSize]byte
		binary.LittleEndian.PutUint32(shdr[0:], names[i].off)
		binary.LittleEndian.PutUint32(shdr[4:], a.sec.typ)
		binary.LittleEndian.PutUint64(shdr[8:], a.sec.flags)
		binary.LittleEndian.PutUint64(shdr[16:], 0)
		binary.LittleEndian.PutUint64(shdr[24:], a.offset)
		binary.LittleEndian.PutUint64(shdr[32:], uint64(len(a.sec.data)))
		binary.LittleEndian.PutUint32(shdr[40:], a.sec.link)
		binary.LittleEndian.PutUint32(shdr[44:], a.sec.info)
		binary.LittleEndian.PutUint64(shdr[48:], a.sec.align)
		binary.LittleEndian.PutUint64(shdr[56:], a.sec.entsize)
		out = append(out, shdr[:]...)
	}

	// 7. Update ELF header
	newShnum := int(e_shnum) + len(added)
	binary.LittleEndian.PutUint16(out[0x3c:], uint16(newShnum))
	binary.LittleEndian.PutUint64(out[0x28:], allShdrOff)

	// Write output
	if err := os.WriteFile(outputFile, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}

	textModCount := 0
	relaModCount := 0
	for _, a := range added {
		if strings.HasPrefix(a.sec.name, ".modtext.") {
			textModCount++
		}
		if strings.HasPrefix(a.sec.name, ".rela.modtext.") {
			relaModCount++
		}
	}
	fmt.Fprintf(os.Stderr, "elfsplit: wrote %s (%d bytes, %d sections, +%d .text.mod + %d .rela.text.mod)\n",
		outputFile, len(out), newShnum, textModCount, relaModCount)
}

// rewritePCRelativeBranches fixes direct branches whose displacement was
// resolved by the Go linker before elfsplit saw the object. The returned
// relocations cover raw branches whose target must be resolved by lld after
// the source and target live in different sections.
func rewritePCRelativeBranches(
	data, destination []byte,
	textOff uint64,
	fn codePlacement,
	placements []codePlacement,
	textRelocOffsets map[uint64]struct{},
	textSize uint64,
) ([]byte, error) {
	var synthetic []byte
	fnStart := textOff + fn.oldOff
	fnEnd := fnStart + fn.size
	if fnEnd > uint64(len(data)) || fn.newOff+fn.size > uint64(len(destination)) {
		return nil, fmt.Errorf("function %q range out of bounds", fn.name)
	}
	for pos := uint64(0); pos < fn.size; {
		inst, err := x86asm.Decode(data[fnStart+pos:fnEnd], 64)
		if err != nil || inst.Len <= 0 || pos+uint64(inst.Len) > fn.size {
			// Keep scanning after an undecodable byte. The symbol size can
			// include bytes which are not part of a decodable instruction
			// stream, but one bad byte must not hide later branches.
			pos++
			continue
		}

		if inst.PCRel == 0 {
			pos += uint64(inst.Len)
			continue
		}

		// PCRel covers both direct branches (x86asm.Rel) and RIP-relative
		// memory operands (e.g. leaq sym(%rip),%rax / movq sym(%rip),%rax),
		// which Go uses for open-coded defer wrappers and other function
		// address loads. Both encode target = nextIP + disp; the displacement
		// itself lives in the Rel argument or the RIP-relative Mem operand.
		var disp int64
		hasDisp := false
		for _, arg := range inst.Args {
			switch v := arg.(type) {
			case x86asm.Rel:
				disp, hasDisp = int64(v), true
			case x86asm.Mem:
				if v.Base == x86asm.RIP {
					disp, hasDisp = v.Disp, true
				}
			}
			if hasDisp {
				break
			}
		}
		if !hasDisp {
			pos += uint64(inst.Len)
			continue
		}

		// A relocation already present in .rela.text is moved to the
		// corresponding output section and must be left for lld to apply.
		relocOffset := fn.oldOff + pos + uint64(inst.PCRelOff)
		if _, exists := textRelocOffsets[relocOffset]; exists {
			pos += uint64(inst.Len)
			continue
		}

		oldTargetSigned := int64(fn.oldOff) + int64(pos) + int64(inst.Len) + disp
		if oldTargetSigned < 0 || uint64(oldTargetSigned) >= textSize {
			pos += uint64(inst.Len)
			continue
		}
		oldTarget := uint64(oldTargetSigned)
		target := findPlacement(placements, oldTarget)
		// The Go linker resolves jumps to labels inside a function without an
		// ELF relocation, and those labels do not have standalone symbols.
		// The containing function is nevertheless enough to relocate the
		// branch when the source remains in the same output section.
		if target == nil && oldTarget >= fn.oldOff && oldTarget < fn.oldOff+fn.size {
			target = &fn
		}
		if target == nil {
			// An undecodable byte can make the byte-by-byte recovery scan
			// recognize a false branch in padding or in an instruction form
			// unsupported by x86asm. A real cross-function branch has a
			// target function placement (or an ELF relocation), so leave this
			// unrecognized candidate untouched.
			pos += uint64(inst.Len)
			continue
		}

		if target.module == fn.module {
			newTarget := int64(target.newOff + oldTarget - target.oldOff)
			newPC := int64(fn.newOff + pos + uint64(inst.Len))
			if err := writePCRelativeValue(destination, fn.newOff+pos+uint64(inst.PCRelOff), inst.PCRel, newTarget-newPC); err != nil {
				return nil, fmt.Errorf("%q at %#x: %w", fn.name, fn.oldOff+pos, err)
			}
		} else {
			// The branch now crosses an ELF section boundary. Re-express it
			// as a relocation against the target function; the addend keeps
			// the original intra-function target offset.
			reloc, err := makePCRelativeRelocation(
				fn.newOff+pos+uint64(inst.PCRelOff), target.symIdx, inst.PCRel,
				int64(oldTarget-target.oldOff),
			)
			if err != nil {
				return nil, fmt.Errorf("%q at %#x: %w", fn.name, fn.oldOff+pos, err)
			}
			synthetic = append(synthetic, reloc...)
		}
		pos += uint64(inst.Len)
	}
	return synthetic, nil
}

func findSymbol(symbols []symMeta, name string) (symMeta, bool) {
	for _, sym := range symbols {
		if sym.name == name {
			return sym, true
		}
	}
	return symMeta{}, false
}

func alignUp(value, alignment uint64) uint64 {
	if alignment <= 1 {
		return value
	}
	rem := value % alignment
	if rem == 0 {
		return value
	}
	return value + alignment - rem
}

func alignFileOffset(out *[]byte, offset, alignment uint64) uint64 {
	if alignment <= 1 {
		return offset
	}
	rem := offset % alignment
	if rem == 0 {
		return offset
	}
	padding := alignment - rem
	*out = append(*out, make([]byte, int(padding))...)
	return offset + padding
}

// buildKeepChunks returns the portions of the original .text which are not
// occupied by module functions. Overlapping/alias symbols are merged here so
// an alias cannot make a removed range reappear as a retained gap.
func buildKeepChunks(textSize uint64, moved []modFunc) []keepChunk {
	var chunks []keepChunk
	var cursor uint64
	for _, r := range moved {
		if r.size == 0 || r.oldOff >= textSize {
			continue
		}
		end := r.oldOff + r.size
		if end < r.oldOff || end > textSize {
			end = textSize
		}
		if r.oldOff > cursor {
			chunks = append(chunks, keepChunk{oldOff: cursor, size: r.oldOff - cursor})
		}
		if end > cursor {
			cursor = end
		}
	}
	if cursor < textSize {
		chunks = append(chunks, keepChunk{oldOff: cursor, size: textSize - cursor})
	}
	return chunks
}

func findKeepChunk(chunks []keepChunk, target uint64) *keepChunk {
	idx := sort.Search(len(chunks), func(i int) bool {
		return chunks[i].oldOff > target
	})
	for i := idx - 1; i >= 0; i-- {
		chunk := &chunks[i]
		if target < chunk.oldOff {
			continue
		}
		if target < chunk.oldOff+chunk.size {
			return chunk
		}
		break
	}
	return nil
}

func findPlacement(placements []codePlacement, target uint64) *codePlacement {
	idx := sort.Search(len(placements), func(i int) bool {
		return placements[i].oldOff > target
	})
	for i := idx - 1; i >= 0; i-- {
		placement := &placements[i]
		if placement.oldOff > target {
			continue
		}
		end := placement.oldOff + placement.size
		if end >= placement.oldOff && target < end {
			return placement
		}
		if end <= target {
			break
		}
	}
	return nil
}

func findAddedSection(sections []addedSec, name string) int {
	for _, section := range sections {
		if section.sec.name == name {
			return section.newIdx
		}
	}
	return -1
}

func writeSymbolPlacement(out []byte, symtabOff uint64, symIdx, sectionIdx int, value uint64) {
	rawIdx := symIdx + 1
	off := symtabOff + uint64(rawIdx)*elf64SymSize
	if rawIdx < 1 || off+elf64SymSize > uint64(len(out)) {
		panic(fmt.Sprintf("elfsplit: symbol index %d out of bounds", symIdx))
	}
	binary.LittleEndian.PutUint16(out[off+6:], uint16(sectionIdx))
	binary.LittleEndian.PutUint64(out[off+8:], value)
}

func writeSymbolSize(out []byte, symtabOff uint64, symIdx int, size uint64) {
	rawIdx := symIdx + 1
	off := symtabOff + uint64(rawIdx)*elf64SymSize
	if rawIdx < 1 || off+elf64SymSize > uint64(len(out)) {
		panic(fmt.Sprintf("elfsplit: symbol index %d out of bounds", symIdx))
	}
	binary.LittleEndian.PutUint64(out[off+16:], size)
}

func buildTextMapEntries(keepChunks []keepChunk, modGroups map[string][]modFunc, modules []string, textSymIdx int, originalTextSize uint64, etextSymIdx int) []textMapEntry {
	entries := make([]textMapEntry, 0, len(keepChunks))
	for _, chunk := range keepChunks {
		if chunk.size == 0 {
			continue
		}
		entries = append(entries, textMapEntry{
			vaddr:  chunk.oldOff,
			end:    chunk.oldOff + chunk.size,
			symIdx: textSymIdx,
			addend: int64(chunk.newOff),
		})
	}
	for _, mod := range modules {
		for _, fn := range modGroups[mod] {
			if fn.size == 0 {
				continue
			}
			entries = append(entries, textMapEntry{
				vaddr:  fn.oldOff,
				end:    fn.oldOff + fn.size,
				symIdx: fn.symIdx,
				// writeSymbolPlacement sets the function symbol's value to its
				// packed offset inside .modtext.<mod>; an absolute relocation
				// with addend 0 therefore resolves baseaddr to the function's
				// actual physical address (section base + packed offset).
				addend: 0,
			})
		}
	}
	// Sentinel entry for the end of the original text. The pclntab ftab ends
	// with entryoff = original text size, and moduledataverify computes
	// maxpc = textAddr(sentinel). Mapping the original end to runtime.etext
	// keeps the table sorted and lets maxpc cover the packed .text plus every
	// retained .modtext.* section.
	entries = append(entries, textMapEntry{
		vaddr:  originalTextSize,
		end:    originalTextSize + 1,
		symIdx: etextSymIdx,
		addend: 0,
	})
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].vaddr == entries[j].vaddr {
			return entries[i].end < entries[j].end
		}
		return entries[i].vaddr < entries[j].vaddr
	})
	return entries
}

func encodeTextMap(entries []textMapEntry) ([]byte, []byte) {
	data := make([]byte, len(entries)*textSectionMapEntrySize)
	relas := make([]byte, 0, len(entries)*elf64RelaSize)
	for i, entry := range entries {
		base := i * textSectionMapEntrySize
		binary.LittleEndian.PutUint64(data[base:], entry.vaddr)
		binary.LittleEndian.PutUint64(data[base+8:], entry.end)
		var rela [elf64RelaSize]byte
		binary.LittleEndian.PutUint64(rela[0:], uint64(base+16))
		rInfo := (uint64(uint32(entry.symIdx+1)) << 32) | uint64(elf.R_X86_64_64)
		binary.LittleEndian.PutUint64(rela[8:], rInfo)
		binary.LittleEndian.PutUint64(rela[16:], uint64(entry.addend))
		relas = append(relas, rela[:]...)
	}
	return data, relas
}

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func findMovedRange(ranges []modFunc, target uint64) *modFunc {
	idx := sort.Search(len(ranges), func(i int) bool {
		return ranges[i].oldOff > target
	})
	if idx == 0 {
		return nil
	}
	candidate := &ranges[idx-1]
	if target >= candidate.oldOff && target < candidate.oldOff+candidate.size {
		return candidate
	}
	return nil
}

func findTextFunction(symbols []symMeta, target uint64) *symMeta {
	idx := sort.Search(len(symbols), func(i int) bool {
		return symbols[i].value > target
	})
	if idx == 0 {
		return nil
	}
	candidate := &symbols[idx-1]
	if target >= candidate.value && target < candidate.value+candidate.size {
		return candidate
	}
	return nil
}

func symbolByIndex(symbols []symMeta, index int) *symMeta {
	for i := range symbols {
		if symbols[i].idx == index {
			return &symbols[i]
		}
	}
	return nil
}

func writePCRelativeValue(data []byte, offset uint64, width int, value int64) error {
	if offset+uint64(width) > uint64(len(data)) {
		return fmt.Errorf("PC-relative immediate out of bounds")
	}
	switch width {
	case 1:
		if value < -128 || value > 127 {
			return fmt.Errorf("PC-relative displacement %d overflows int8", value)
		}
		data[offset] = byte(int8(value))
	case 2:
		if value < -32768 || value > 32767 {
			return fmt.Errorf("PC-relative displacement %d overflows int16", value)
		}
		binary.LittleEndian.PutUint16(data[offset:], uint16(int16(value)))
	case 4:
		if value < -1<<31 || value > 1<<31-1 {
			return fmt.Errorf("PC-relative displacement %d overflows int32", value)
		}
		binary.LittleEndian.PutUint32(data[offset:], uint32(int32(value)))
	default:
		return fmt.Errorf("unsupported PC-relative width %d", width)
	}
	return nil
}

func makePCRelativeRelocation(offset uint64, symIdx, width int, targetDelta int64) ([]byte, error) {
	var relocType elf.R_X86_64
	switch width {
	case 1:
		relocType = elf.R_X86_64_PC8
	case 2:
		relocType = elf.R_X86_64_PC16
	case 4:
		relocType = elf.R_X86_64_PC32
	default:
		return nil, fmt.Errorf("unsupported PC-relative relocation width %d", width)
	}
	var entry [elf64RelaSize]byte
	binary.LittleEndian.PutUint64(entry[0:], offset)
	// symIdx comes from debug/elf.File.Symbols(), which omits the ELF null
	// symbol at raw index 0. Relocation r_info uses the raw symbol-table index.
	rInfo := (uint64(uint32(symIdx+1)) << 32) | uint64(relocType)
	binary.LittleEndian.PutUint64(entry[8:], rInfo)
	// ELF's relocation place is the immediate field, while x86 relative
	// branches measure from the next instruction (immediate width bytes later).
	binary.LittleEndian.PutUint64(entry[16:], uint64(targetDelta-int64(width)))
	return entry[:], nil
}

func classifyPackage(symName string) string {
	lastDot := strings.LastIndexByte(symName, '.')
	if lastDot < 0 {
		return "unknown"
	}
	pkg := symName[:lastDot]
	if idx := strings.IndexByte(pkg, '('); idx >= 0 {
		beforeParen := pkg[:idx]
		lastDotBefore := strings.LastIndexByte(beforeParen, '.')
		if lastDotBefore >= 0 {
			pkg = beforeParen[:lastDotBefore]
		}
	}
	return pkg
}

func buildModulePackageMap(modules []string) map[string][]string {
	knownPaths := map[string][]string{
		"poc":        {"github.com/yaklang/yaklang/common/utils/lowhttp/poc"},
		"cli":        {"github.com/yaklang/yaklang/common/utils/cli"},
		"ssa":        {"github.com/yaklang/yaklang/common/yak/ssaapi", "github.com/yaklang/yaklang/common/yak/ssaproject", "github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig", "github.com/yaklang/yaklang/common/yak/ssa"},
		"syntaxflow": {"github.com/yaklang/yaklang/common/syntaxflow", "github.com/yaklang/yaklang/common/yak/syntaxflow_scan"},
		"sfreport":   {"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"},
		"http":       {"github.com/yaklang/yaklang/common/yak/yaklib/yakhttp"},
		"yakit":      {"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/shim"},
		"tools":      {"github.com/yaklang/yaklang/common/yak/yaklib/tools"},
		"crawler":    {"github.com/yaklang/yaklang/common/crawler"},
		"yso":        {"github.com/yaklang/yaklang/common/yso"},
		"facades":    {"github.com/yaklang/yaklang/common/facades"},
		"nuclei":     {"github.com/yaklang/yaklang/common/yak/httptpl"},
		"httptpl":    {"github.com/yaklang/yaklang/common/yak/httptpl"},
		"ai":         {"github.com/yaklang/yaklang/common/ai"},
		"liteforge":  {"github.com/yaklang/yaklang/common/aiforge"},
		"hids":       {"github.com/yaklang/yaklang/common/hids"},
		"java":       {"github.com/yaklang/yaklang/common/yserx"},
		"t3":         {"github.com/yaklang/yaklang/common/t3"},
		"iiop":       {"github.com/yaklang/yaklang/common/iiop"},
		"jwt":        {"github.com/yaklang/yaklang/common/authhack"},
	}
	result := make(map[string][]string)
	for _, mod := range modules {
		if paths, ok := knownPaths[mod]; ok {
			result[mod] = paths
		}
	}
	return result
}

// matchRegisterSymbol checks if a symbol is a module registration function.
func matchRegisterSymbol(symName string, modules []string) string {
	for _, mod := range modules {
		if strings.Contains(symName, "yak_register_module_"+mod) {
			return mod
		}
	}
	return ""
}

// matchRegisterSymbol checks if a symbol is a module registration function.

func matchModule(pkg string, modulePkgs map[string][]string) string {
	for mod, paths := range modulePkgs {
		for _, p := range paths {
			if pkg == p || strings.HasPrefix(pkg, p+"/") || strings.HasPrefix(pkg, p+".") {
				return mod
			}
		}
	}
	return ""
}
