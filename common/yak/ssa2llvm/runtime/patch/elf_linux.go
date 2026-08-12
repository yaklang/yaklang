//go:build linux

package patch

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

const (
	elf64SymSize  = 24
	elf64RelaSize = 24
)

// elfSection is a parsed ELF section header.
type elfSection struct {
	name   string
	idx    int
	typ    uint32
	flags  uint64
	addr   uint64
	offset uint64
	size   uint64
	link   uint32
	info   uint32
}

// parseELFSections reads section headers and returns them plus the shstrtab.
func parseELFSections(data []byte) ([]elfSection, []byte, uint16, uint16, error) {
	if len(data) < 0x40 || string(data[:4]) != "\x7fELF" {
		return nil, nil, 0, 0, fmt.Errorf("not an ELF file")
	}
	if data[4] != 2 { // ELF64
		return nil, nil, 0, 0, fmt.Errorf("only ELF64 supported")
	}
	e_shoff := binary.LittleEndian.Uint64(data[0x28:])
	e_shentsize := binary.LittleEndian.Uint16(data[0x3a:])
	e_shnum := binary.LittleEndian.Uint16(data[0x3c:])
	e_shstrndx := binary.LittleEndian.Uint16(data[0x3e:])
	if e_shentsize < 64 || e_shnum == 0 || int(e_shoff)+int(e_shnum)*64 > len(data) {
		return nil, nil, 0, 0, fmt.Errorf("bad ELF section header table")
	}
	sections := make([]elfSection, 0, e_shnum)
	for i := 0; i < int(e_shnum); i++ {
		off := int(e_shoff) + i*64
		sections = append(sections, elfSection{
			name:   "",
			idx:    i,
			typ:    binary.LittleEndian.Uint32(data[off+4:]),
			flags:  binary.LittleEndian.Uint64(data[off+8:]),
			addr:   binary.LittleEndian.Uint64(data[off+16:]),
			offset: binary.LittleEndian.Uint64(data[off+24:]),
			size:   binary.LittleEndian.Uint64(data[off+32:]),
			link:   binary.LittleEndian.Uint32(data[off+40:]),
			info:   binary.LittleEndian.Uint32(data[off+44:]),
		})
	}
	if int(e_shstrndx) >= len(sections) {
		return nil, nil, 0, 0, fmt.Errorf("bad shstrtab index")
	}
	shstr := sections[e_shstrndx]
	if int(shstr.offset)+int(shstr.size) > len(data) {
		return nil, nil, 0, 0, fmt.Errorf("shstrtab out of range")
	}
	shstrData := data[shstr.offset : shstr.offset+shstr.size]
	for i := range sections {
		nameOff := binary.LittleEndian.Uint32(data[int(e_shoff)+i*64:])
		if int(nameOff) < len(shstrData) {
			end := bytes.IndexByte(shstrData[nameOff:], 0)
			if end >= 0 {
				sections[i].name = string(shstrData[nameOff : nameOff+uint32(end)])
			}
		}
	}
	return sections, shstrData, e_shentsize, e_shnum, nil
}

// findModtextModules returns the set of module names with a .modtext.<m> section.
func findModtextModules(sections []elfSection) []string {
	used := map[string]bool{}
	var out []string
	for _, s := range sections {
		if strings.HasPrefix(s.name, ".modtext.") {
			m := strings.TrimPrefix(s.name, ".modtext.")
			if m != "" && !used[m] {
				used[m] = true
				out = append(out, m)
			}
		}
	}
	return out
}

// collectModtextSymbols returns symtab indices of symbols in .modtext.<module> sections.
func collectModtextSymbols(data []byte, sections []elfSection, onlyModules []string) (map[uint32]string, error) {
	var symtab *elfSection
	var strtab *elfSection
	for i := range sections {
		if sections[i].name == ".symtab" {
			symtab = &sections[i]
		}
		if symtab != nil && sections[i].idx == int(symtab.link) {
			strtab = &sections[i]
		}
	}
	if symtab == nil || strtab == nil {
		return nil, fmt.Errorf("symtab/strtab not found")
	}
	if int(symtab.offset)+int(symtab.size) > len(data) {
		return nil, fmt.Errorf("symtab out of range")
	}
	modtextSec := map[uint16]bool{}
	if len(onlyModules) == 0 {
		// 未指定时收集全部 .modtext.*
		for _, s := range sections {
			if strings.HasPrefix(s.name, ".modtext.") {
				modtextSec[uint16(s.idx)] = true
			}
		}
	} else {
		only := map[string]bool{}
		for _, m := range onlyModules {
			if m != "" {
				only[m] = true
			}
		}
		for _, s := range sections {
			if strings.HasPrefix(s.name, ".modtext.") {
				m := strings.TrimPrefix(s.name, ".modtext.")
				if only[m] {
					modtextSec[uint16(s.idx)] = true
				}
			}
		}
	}
	str := data[strtab.offset : strtab.offset+strtab.size]
	count := int(symtab.size) / elf64SymSize
	modtextSyms := map[uint32]string{}
	for i := 0; i < count; i++ {
		off := int(symtab.offset) + i*elf64SymSize
		shndx := binary.LittleEndian.Uint16(data[off+6:])
		if !modtextSec[shndx] {
			continue
		}
		nameOff := binary.LittleEndian.Uint32(data[off:])
		name := ""
		if int(nameOff) < len(str) {
			end := bytes.IndexByte(str[nameOff:], 0)
			if end >= 0 {
				name = string(str[nameOff : nameOff+uint32(end)])
			}
		}
		// symbol table index i is the RAW index (0 = null), matching r_info>>32.
		modtextSyms[uint32(i)] = name
	}
	return modtextSyms, nil
}

// neutralizeModtextRelocs redirects RELA entries that referenced removed
// .modtext.<module> symbols to the retained yakUnusedModuleStub. Zeroing the
// relocation entries is not safe: a PC-relative relocation in retained code
// then computes a garbage address (for example an LEA that materializes a
// function pointer), which later crashes on an indirect call. Pointing every
// such reference at a no-op stub keeps absolute data slots, PC-relative code,
// and function tables safe while lld still drops the module section itself.
func neutralizeModtextRelocs(data []byte, sections []elfSection, modtextSyms map[uint32]string, removedModules map[string]bool) (int, error) {
	if len(modtextSyms) == 0 {
		return 0, nil
	}
	stubRaw, err := findRawSymbol(data, sections, "main.yakUnusedModuleStub")
	if err != nil {
		// Older archives built before the stub existed cannot redirect; fall
		// back to the previous zeroing behavior so they still link. The zeroed
		// PC-relative slots are unsafe at runtime, which is why new archives
		// include the stub, but a fallback keeps the tool usable with old
		// embedded assets during migration/baseline checks.
		stubRaw = 0
	}
	total := 0
	for _, s := range sections {
		if s.typ != uint32(elf.SHT_RELA) || s.size == 0 {
			continue
		}
		if strings.HasPrefix(s.name, ".rela.modtext.") {
			// Relocations inside a removed module section are discarded
			// together with that section by lld GC. But a USED module section
			// (e.g. .rela.modtext.ssa) can contain references into a removed
			// module (the language frontends); those must be redirected below
			// or lld keeps the removed section alive.
			if int(s.info) < len(sections) {
				target := sections[s.info].name
				if strings.HasPrefix(target, ".modtext.") && removedModules[strings.TrimPrefix(target, ".modtext.")] {
					continue
				}
			}
		}
		if s.name == ".rela.data.rel.ro.yaktextmap" {
			// The runtime text map must not keep unused module sections live.
			// Zero its relocations (as before), leaving baseaddr=0 entries
			// that runtime lookup never reaches for unretained modules.
			if int(s.offset)+int(s.size) > len(data) {
				return total, fmt.Errorf("rela section %s out of range", s.name)
			}
			count := int(s.size) / elf64RelaSize
			for i := 0; i < count; i++ {
				off := int(s.offset) + i*elf64RelaSize
				rInfo := binary.LittleEndian.Uint64(data[off+8:])
				rSym := uint32(rInfo >> 32)
				if _, ok := modtextSyms[rSym]; ok {
					for k := 0; k < elf64RelaSize; k++ {
						data[off+k] = 0
					}
					total++
				}
			}
			continue
		}
		if int(s.offset)+int(s.size) > len(data) {
			return total, fmt.Errorf("rela section %s out of range", s.name)
		}
		count := int(s.size) / elf64RelaSize
		for i := 0; i < count; i++ {
			off := int(s.offset) + i*elf64RelaSize
			rInfo := binary.LittleEndian.Uint64(data[off+8:])
			rSym := uint32(rInfo >> 32)
			if _, ok := modtextSyms[rSym]; !ok {
				continue
			}
			if stubRaw == 0 {
				for k := 0; k < elf64RelaSize; k++ {
					data[off+k] = 0
				}
				total++
				continue
			}
			newInfo := (uint64(stubRaw) << 32) | (rInfo & 0xffffffff)
			binary.LittleEndian.PutUint64(data[off+8:], newInfo)
			// x86-64 PC-relative relocations resolve to S + A + 4 (the
			// displacement field is relative to the end of the instruction),
			// so a zero addend would point four bytes past the stub into its
			// int3 padding. Compensate per relocation width; absolute
			// relocations keep addend 0.
			switch elf.R_X86_64(rInfo & 0xffffffff) {
			case elf.R_X86_64_PC32, elf.R_X86_64_PLT32:
				binary.LittleEndian.PutUint64(data[off+16:], ^uint64(3))
			case elf.R_X86_64_PC16:
				binary.LittleEndian.PutUint64(data[off+16:], ^uint64(1))
			case elf.R_X86_64_PC8:
				binary.LittleEndian.PutUint64(data[off+16:], ^uint64(0))
			default:
				binary.LittleEndian.PutUint64(data[off+16:], 0)
			}
			total++
		}
	}
	return total, nil
}

func findRawSymbol(data []byte, sections []elfSection, name string) (uint32, error) {
	var symtab *elfSection
	var strtab *elfSection
	for i := range sections {
		if sections[i].name == ".symtab" {
			symtab = &sections[i]
		}
		if symtab != nil && sections[i].idx == int(symtab.link) {
			strtab = &sections[i]
		}
	}
	if symtab == nil || strtab == nil {
		return 0, fmt.Errorf("symtab/strtab not found")
	}
	str := data[strtab.offset : strtab.offset+strtab.size]
	count := int(symtab.size) / elf64SymSize
	for i := 0; i < count; i++ {
		off := int(symtab.offset) + i*elf64SymSize
		nameOff := binary.LittleEndian.Uint32(data[off:])
		if int(nameOff) >= len(str) {
			continue
		}
		end := bytes.IndexByte(str[nameOff:], 0)
		if end < 0 {
			continue
		}
		if string(str[nameOff:nameOff+uint32(end)]) == name {
			return uint32(i), nil
		}
	}
	return 0, fmt.Errorf("symbol %q not found", name)
}

// markInittasksDone sets state=2 for init tasks whose function relocation
// points into a removed .modtext section. Matching the relocation target is
// intentional: the yaklib module name is not always the package basename
// (for example, module "yakit" is implemented by package "runtime/shim"),
// and composite modules can contain several init tasks.
func markInittasksDone(data []byte, sections []elfSection, modtextSyms map[uint32]string) (int, error) {
	if len(modtextSyms) == 0 {
		return 0, nil
	}
	var symtab *elfSection
	var strtab *elfSection
	for i := range sections {
		if sections[i].name == ".symtab" {
			symtab = &sections[i]
		}
		if symtab != nil && sections[i].idx == int(symtab.link) {
			strtab = &sections[i]
		}
	}
	if symtab == nil || strtab == nil {
		return 0, fmt.Errorf("symtab/strtab not found")
	}
	str := data[strtab.offset : strtab.offset+strtab.size]
	count := int(symtab.size) / elf64SymSize
	type initTask struct {
		section uint16
		value   uint64
		size    uint64
	}
	var tasks []initTask
	for i := 0; i < count; i++ {
		off := int(symtab.offset) + i*elf64SymSize
		nameOff := binary.LittleEndian.Uint32(data[off:])
		if int(nameOff) >= len(str) {
			continue
		}
		end := bytes.IndexByte(str[nameOff:], 0)
		if end < 0 {
			continue
		}
		name := string(str[nameOff : nameOff+uint32(end)])
		if !strings.HasSuffix(name, "..inittask") {
			continue
		}
		shndx := binary.LittleEndian.Uint16(data[off+6:])
		if int(shndx) >= len(sections) {
			continue
		}
		tasks = append(tasks, initTask{
			section: shndx,
			value:   binary.LittleEndian.Uint64(data[off+8:]),
			size:    binary.LittleEndian.Uint64(data[off+16:]),
		})
	}

	// Relocation offsets are relative to the section named by sh_info. A
	// package init task stores its init function pointer inside the task
	// object, so a relocation into a removed .modtext function identifies the
	// exact task even when its package path does not match the yaklib module
	// name.
	doneTasks := make(map[int]struct{})
	for _, relaSec := range sections {
		if relaSec.typ != uint32(elf.SHT_RELA) || relaSec.size == 0 || int(relaSec.info) >= len(sections) {
			continue
		}
		targetSection := uint16(relaSec.info)
		if int(relaSec.offset)+int(relaSec.size) > len(data) {
			return len(doneTasks), fmt.Errorf("rela section %s out of range", relaSec.name)
		}
		count := int(relaSec.size) / elf64RelaSize
		for i := 0; i < count; i++ {
			off := int(relaSec.offset) + i*elf64RelaSize
			rOffset := binary.LittleEndian.Uint64(data[off:])
			rInfo := binary.LittleEndian.Uint64(data[off+8:])
			if _, ok := modtextSyms[uint32(rInfo>>32)]; !ok {
				continue
			}
			for taskIdx, task := range tasks {
				if task.section != targetSection || rOffset < task.value || rOffset >= task.value+task.size {
					continue
				}
				sec := sections[task.section]
				loc := int(sec.offset + task.value)
				if loc+4 > len(data) {
					return len(doneTasks), fmt.Errorf("inittask out of range at %#x", sec.offset+task.value)
				}
				data[loc] = 2 // state = fully initialized
				doneTasks[taskIdx] = struct{}{}
				break
			}
		}
	}

	done := len(doneTasks)
	return done, nil
}

// patchELF mutates data in place, returning removed reloc count.
func patchELF(data []byte, usedModules []string) (int, error) {
	sections, _, _, _, err := parseELFSections(data)
	if err != nil {
		return 0, err
	}
	modtext := findModtextModules(sections)
	if len(modtext) == 0 {
		return 0, nil // no split modules; nothing to do
	}
	// modules to remove = all modtext - used
	used := map[string]bool{}
	for _, m := range usedModules {
		used[m] = true
	}
	var toRemove []string
	for _, m := range modtext {
		if !used[m] {
			toRemove = append(toRemove, m)
		}
	}
	if len(toRemove) == 0 {
		return 0, nil
	}
	modtextSyms, err := collectModtextSymbols(data, sections, toRemove)
	if err != nil {
		return 0, err
	}
	if len(modtextSyms) == 0 {
		return 0, nil
	}
	marked, err := markInittasksDone(data, sections, modtextSyms)
	if err != nil {
		return marked, err
	}
	removedModules := map[string]bool{}
	for _, m := range toRemove {
		removedModules[m] = true
	}
	removed, err := neutralizeModtextRelocs(data, sections, modtextSyms, removedModules)
	if err != nil {
		return removed, err
	}
	return removed, nil
}

// patchArchive mutates every ELF member of the ar archive in place.
// Zeroing relocations and setting inittask.state do not change any length,
// so the archive layout is preserved byte-for-byte.
func patchArchive(path string, usedModules []string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read archive: %w", err)
	}
	if len(raw) < 8 || string(raw[:8]) != "!<arch>\n" {
		return 0, fmt.Errorf("not a GNU ar archive")
	}
	pos := 8
	total := 0
	for pos+60 <= len(raw) {
		hdr := raw[pos : pos+60]
		sizeStr := strings.TrimRight(string(hdr[48:58]), " ")
		var size int
		for _, cc := range sizeStr {
			if cc < '0' || cc > '9' {
				return total, fmt.Errorf("bad ar size %q", sizeStr)
			}
			size = size*10 + int(cc-'0')
		}
		dataStart := pos + 60
		if dataStart+size > len(raw) {
			return total, fmt.Errorf("truncated ar member")
		}
		payload := raw[dataStart : dataStart+size]
		if len(payload) > 4 && string(payload[:4]) == "\x7fELF" {
			n, err := patchELF(payload, usedModules)
			if err != nil {
				return total, fmt.Errorf("patch ELF member: %w", err)
			}
			total += n
		}
		advance := 60 + size
		if size%2 == 1 {
			advance++
		}
		pos += advance
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return total, fmt.Errorf("write archive: %w", err)
	}
	return total, nil
}
