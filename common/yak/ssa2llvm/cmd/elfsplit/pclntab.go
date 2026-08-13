package main

// Function-name folding for the Go pclntab.
//
// About a third of .gopclntab is funcnametab: the concatenated names of every
// function in the module. It is the only sub-table the runtime never needs —
// stack unwinding, GC stack scanning, panic/recover and preemption all read
// ftab/_func/pctab/funcdata, while funcnametab only turns a PC into a printable
// name. In an AOT artifact whose code was already pruned per script, those names
// describe mostly-deleted code: the language frontends alone account for the
// majority of them.
//
// foldFuncNameTable removes the table's bytes from the file and replaces it with
// a same-sized SHT_NOBITS region, then clears every _func.nameOff. The result:
//
//   - runtime.funcName(0) short-circuits to "" without a memory access, so
//     normal frames symbolize as an empty name;
//   - inlined frames read their name offset from the inline tree, which this
//     pass cannot enumerate. They index the NOBITS region instead, which is
//     zero-filled, so they also produce "" — in bounds, never faulting;
//   - GC, stack growth, traceback structure and moduledataverify are untouched.
//
// The cost is that a traceback shows no function names. Keep the unfolded
// archive around (or rebuild with folding off) when a stack trace has to be
// read.

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
)

// pcHeader field offsets (runtime/symtab.go). The five *Offset fields are
// relative to the start of pcHeader and are written by the Go linker; the
// runtime itself never reads them, which is what makes the region movable.
const (
	phMagic          = 0
	phPad1           = 4
	phPad2           = 5
	phMinLC          = 6
	phPtrSize        = 7
	phNfunc          = 8
	phNfiles         = 16
	phFuncnameOffset = 32
	phCuOffset       = 40
	phFiletabOffset  = 48
	phPctabOffset    = 56
	phPclnOffset     = 64

	// pclntabMagic is abi.CurrentPCLnTabMagic. A mismatch means the table
	// layout assumed here no longer holds.
	pclntabMagic = 0xFFFFFFF1
)

// moduledata field offsets (runtime/symtab.go). Only the leading slice headers
// are touched; every one of them is cross-checked against the pcHeader before
// anything is written.
const (
	mdFuncnametab = 8
	mdCutab       = 32
	mdFiletab     = 56
	mdPctab       = 80
	mdPclntable   = 104
	mdFtab        = 128

	sliceLenOffset = 8
	sliceCapOffset = 16

	functabEntrySize = 8 // functab{entryoff, funcoff uint32}
	funcNameOffField = 4 // _func.nameOff
)

const foldedNamesSectionName = ".yakfuncnames"

// foldedNamesSymbol is the symbol moduledata.funcnametab is repointed at. It
// must be global: it is appended after the symbol table's existing locals, and
// ELF requires all local symbols to come first.
const foldedNamesSymbol = "yak.funcnametab.zero"

type foldStats struct {
	tableBytes uint64
	funcs      uint64
}

// foldFuncNameTable rewrites data so that .gopclntab no longer carries
// funcnametab. It returns the rewritten image; data is left untouched.
func foldFuncNameTable(data []byte) ([]byte, foldStats, error) {
	var stats foldStats

	img, err := readELFImage(data)
	if err != nil {
		return nil, stats, err
	}
	pclntabSec := img.sectionByName(".gopclntab")
	symtabSec := img.sectionByName(".symtab")
	goModuleSec := img.sectionByName(".go.module")
	relaGoModuleSec := img.sectionByName(".rela.go.module")
	if pclntabSec < 0 || symtabSec < 0 || goModuleSec < 0 || relaGoModuleSec < 0 {
		return nil, stats, fmt.Errorf("missing .gopclntab/.symtab/.go.module/.rela.go.module")
	}
	strtabSec := int(img.sections[symtabSec].link)
	if strtabSec <= 0 || strtabSec >= len(img.sections) {
		return nil, stats, fmt.Errorf("symtab links to invalid strtab %d", strtabSec)
	}

	pcln := img.sections[pclntabSec].payload
	if uint64(len(pcln)) < phPclnOffset+8 {
		return nil, stats, fmt.Errorf(".gopclntab is too small (%d bytes)", len(pcln))
	}
	if magic := binary.LittleEndian.Uint32(pcln[phMagic:]); magic != pclntabMagic {
		return nil, stats, fmt.Errorf("unexpected pclntab magic %#x (want %#x); the Go pclntab layout changed", magic, pclntabMagic)
	}
	if pcln[phPad1] != 0 || pcln[phPad2] != 0 {
		return nil, stats, fmt.Errorf("pcHeader padding is not zero; the Go pclntab layout changed")
	}
	nfunc := binary.LittleEndian.Uint64(pcln[phNfunc:])
	funcnameOffset := binary.LittleEndian.Uint64(pcln[phFuncnameOffset:])
	cuOffset := binary.LittleEndian.Uint64(pcln[phCuOffset:])
	filetabOffset := binary.LittleEndian.Uint64(pcln[phFiletabOffset:])
	pctabOffset := binary.LittleEndian.Uint64(pcln[phPctabOffset:])
	pclnOffset := binary.LittleEndian.Uint64(pcln[phPclnOffset:])
	if !(funcnameOffset <= cuOffset && cuOffset <= filetabOffset && filetabOffset <= pctabOffset &&
		pctabOffset <= pclnOffset && pclnOffset <= uint64(len(pcln))) {
		return nil, stats, fmt.Errorf("pcHeader sub-table offsets are not monotonic")
	}
	delta := cuOffset - funcnameOffset
	if delta == 0 {
		return nil, stats, nil // already folded, or a module without names
	}

	mdBase, err := img.symbolValue(symtabSec, strtabSec, "runtime.firstmoduledata", goModuleSec)
	if err != nil {
		return nil, stats, err
	}
	modData := img.sections[goModuleSec].payload
	if uint64(len(modData)) < mdBase+mdFtab+sliceCapOffset+8 {
		return nil, stats, fmt.Errorf("runtime.firstmoduledata is truncated")
	}

	// Cross-check the assumed moduledata layout against the pcHeader. Every
	// slice length below is derivable from the header, so a Go release that
	// reorders moduledata is caught here instead of at run time.
	//
	// cutab is []uint32; the rest are byte slices.
	for _, want := range []struct {
		field  uint64
		name   string
		length uint64
	}{
		{mdFuncnametab, "funcnametab", delta},
		{mdCutab, "cutab", (filetabOffset - cuOffset) / 4},
		{mdFiletab, "filetab", pctabOffset - filetabOffset},
		{mdPctab, "pctab", pclnOffset - pctabOffset},
		{mdFtab, "ftab", nfunc + 1},
	} {
		got := binary.LittleEndian.Uint64(modData[mdBase+want.field+sliceLenOffset:])
		if got != want.length {
			return nil, stats, fmt.Errorf("moduledata.%s has length %d, pcHeader implies %d; the Go moduledata layout changed",
				want.name, got, want.length)
		}
	}

	// The Go linker emits every sub-table pointer as runtime.pclntab plus the
	// matching pcHeader offset, so the relocation addends are a second,
	// independent confirmation of the layout — and are what has to move.
	pclntabSym, err := img.rawSymbolIndex(symtabSec, strtabSec, "runtime.pclntab")
	if err != nil {
		return nil, stats, err
	}
	relocs := map[uint64]uint64{
		mdFuncnametab: funcnameOffset,
		mdCutab:       cuOffset,
		mdFiletab:     filetabOffset,
		mdPctab:       pctabOffset,
		mdPclntable:   pclnOffset,
		mdFtab:        pclnOffset,
	}
	relaGoModule := img.sections[relaGoModuleSec].payload
	found := map[uint64]int{}
	for off := 0; off+elf64RelaSize <= len(relaGoModule); off += elf64RelaSize {
		rOffset := binary.LittleEndian.Uint64(relaGoModule[off:])
		if rOffset < mdBase {
			continue
		}
		field := rOffset - mdBase
		wantAddend, ok := relocs[field]
		if !ok {
			continue
		}
		rInfo := binary.LittleEndian.Uint64(relaGoModule[off+8:])
		addend := binary.LittleEndian.Uint64(relaGoModule[off+16:])
		if uint32(rInfo>>32) != pclntabSym || addend != wantAddend {
			return nil, stats, fmt.Errorf("moduledata+%d does not relocate to runtime.pclntab+%d; the Go moduledata layout changed",
				field, wantAddend)
		}
		found[field] = off
	}
	if len(found) != len(relocs) {
		return nil, stats, fmt.Errorf("found %d of %d moduledata pclntab relocations", len(found), len(relocs))
	}

	// Clear _func.nameOff for every function. ftab has nfunc+1 entries; the
	// last is the end sentinel and has no _func behind it.
	newPcln := make([]byte, 0, uint64(len(pcln))-delta)
	newPcln = append(newPcln, pcln[:funcnameOffset]...)
	newPcln = append(newPcln, pcln[cuOffset:]...)
	newPclnOffset := pclnOffset - delta
	for i := uint64(0); i < nfunc; i++ {
		entry := newPclnOffset + i*functabEntrySize
		if entry+functabEntrySize > uint64(len(newPcln)) {
			return nil, stats, fmt.Errorf("ftab entry %d is out of range", i)
		}
		funcoff := uint64(binary.LittleEndian.Uint32(newPcln[entry+4:]))
		nameOff := newPclnOffset + funcoff + funcNameOffField
		if nameOff+4 > uint64(len(newPcln)) {
			return nil, stats, fmt.Errorf("_func for ftab entry %d is out of range", i)
		}
		binary.LittleEndian.PutUint32(newPcln[nameOff:], 0)
		stats.funcs++
	}

	binary.LittleEndian.PutUint64(newPcln[phCuOffset:], funcnameOffset)
	binary.LittleEndian.PutUint64(newPcln[phFiletabOffset:], filetabOffset-delta)
	binary.LittleEndian.PutUint64(newPcln[phPctabOffset:], pctabOffset-delta)
	binary.LittleEndian.PutUint64(newPcln[phPclnOffset:], newPclnOffset)
	img.sections[pclntabSec].payload = newPcln
	img.sections[pclntabSec].size = uint64(len(newPcln))

	// Symbols defined after the removed range move down with it. Nothing may
	// be defined inside the range itself.
	if err := img.shiftSectionSymbols(symtabSec, pclntabSec, funcnameOffset, cuOffset, delta); err != nil {
		return nil, stats, err
	}

	namesSec := img.appendSection(rawSection{
		name:    foldedNamesSectionName,
		typ:     uint32(elf.SHT_NOBITS),
		flags:   uint64(elf.SHF_ALLOC | elf.SHF_WRITE),
		size:    delta,
		align:   8,
		payload: nil,
	})
	namesSym, err := img.appendGlobalObjectSymbol(symtabSec, strtabSec, foldedNamesSymbol, namesSec, delta)
	if err != nil {
		return nil, stats, err
	}

	// Every reference into the table has to follow the bytes that moved. The
	// six moduledata slice headers validated above are not the whole set: the
	// linker also points moduledata.pcHeader at the table's start and
	// moduledata.findfunctab at a region past its end. Rather than enumerate
	// fields, rewrite every relocation against runtime.pclntab by where its
	// addend lands relative to the removed range.
	patched := img.retargetSymbolAddends(pclntabSym, func(addend uint64) (uint32, uint64) {
		switch {
		case addend < funcnameOffset:
			return pclntabSym, addend
		case addend < cuOffset:
			return namesSym, addend - funcnameOffset
		default:
			return pclntabSym, addend - delta
		}
	})
	if patched < len(found) {
		return nil, stats, fmt.Errorf("patched %d runtime.pclntab relocations but validated %d", patched, len(found))
	}

	stats.tableBytes = delta
	return img.write(), stats, nil
}
