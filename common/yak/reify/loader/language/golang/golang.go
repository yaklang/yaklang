package golang

import (
	"bytes"
	"debug/buildinfo"
	"debug/gosym"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// Reading compiler metadata must not allocate the size of an arbitrary input
// section. Large or malformed tables leave the container usable with a note.
// A section the loader names as the Go function table is compiler-owned, so it
// gets a larger ceiling: the yaklang binary alone carries an 85 MiB pclntab,
// and every function name, file name and line table lives in it.
const (
	maxMetadataSize     = 64 << 20
	maxNamedPclntabSize = 512 << 20
)

// metadataLimit returns how large one candidate section may be. A name that
// only looks like a Go table by convention (.data.rel.ro) keeps the small
// limit, because it holds unrelated data too. PE .rdata is the exception: the
// PE linker puts the Go function table there and a large engine exceeds the
// generic ceiling.
func metadataLimit(name string, format core.Format) uint64 {
	if isGoTableSection(name) {
		return maxNamedPclntabSize
	}
	// A PE image has no named Go table: the linker merges .gopclntab into
	// .rdata. A large engine therefore has a .rdata far beyond the generic
	// fallback ceiling, and the function table is in that section.
	if format == core.FormatPE && name == ".rdata" {
		return maxNamedPclntabSize
	}
	return maxMetadataSize
}

type Golang struct{}

func (Golang) Kind() core.Language { return core.LanguageGo }

func (Golang) Detect(a *core.Artifact) *core.LanguageReport {
	r := &core.LanguageReport{Language: core.LanguageGo, Confidence: core.ConfidenceLikely}
	if a.Reader != nil {
		if info, err := buildinfo.Read(a.Reader); err == nil {
			r.Runtime = info.GoVersion
			r.Confidence = core.ConfidenceCertain
			r.Evidence = append(r.Evidence, core.LanguageEvidence{Source: "buildinfo", Detail: info.GoVersion})
		}
	}
	for _, section := range a.Image.Sections {
		if isGoTableSection(section.Name) {
			r.Evidence = append(r.Evidence, core.LanguageEvidence{Source: "section", Detail: section.Name})
		}
	}
	if len(r.Evidence) == 0 {
		return nil
	}
	// The register ABI started on amd64 in Go 1.17, then arm64 in 1.18.
	// Unknown or old versions must not silently acquire a modern register ABI.
	minor := goMinor(r.Runtime)
	if a.Target.Architecture == core.ArchAMD64 && minor >= 17 ||
		a.Target.Architecture == core.ArchARM64 && minor >= 18 {
		r.ABI = goABI(a.Target)
	}
	r.Notes = []string{
		"Go interface dispatch, goroutine scheduling, stack growth, defer/recover and GC semantics are not reconstructed",
		"only scalar integer ABIInternal registers are modelled; ABI0, cross-ABI calls, floats, aggregates and multiple results are not modelled",
	}
	return r
}

// goABI returns the ABIInternal override. ABIInternal passes integers in
// registers chosen by Go, not by the platform convention, and keeps no ordinary
// callee-saved integer registers.
func goABI(target core.Target) *core.ABI {
	registers := goRegisters(target)
	if registers == nil {
		return nil
	}
	return &core.ABI{Registers: *registers, NoCalleeSave: true}
}

// Strings reports the Go string header: a pointer and a length. Go does not
// require valid UTF-8, so the decoder returns the raw bytes.
func (Golang) Strings() core.Strings {
	return core.Strings{Layout: core.StringLayoutPointerLength, Encoding: core.StringEncodingUTF8}
}

func goMinor(version string) int {
	if !strings.HasPrefix(version, "go1.") {
		return 0
	}
	v := strings.TrimPrefix(version, "go1.")
	end := 0
	for end < len(v) && v[end] >= '0' && v[end] <= '9' {
		end++
	}
	n, _ := strconv.Atoi(v[:end])
	return n
}

func goRegisters(target core.Target) *core.RegisterModel {
	switch target.Architecture {
	case core.ArchAMD64:
		return &core.RegisterModel{
			Naming: core.RegisterNamingX86, Width: 64, Flags: "rflags", FlagsWidth: 64,
			Arguments: []string{"rax", "rbx", "rcx", "rdi", "rsi", "r8", "r9", "r10", "r11"}, Return: "rax",
		}
	case core.ArchARM64:
		args := make([]string, 16)
		for i := range args {
			args[i] = fmt.Sprintf("x%d", i)
		}
		return &core.RegisterModel{Naming: core.RegisterNamingAArch64, Width: 64,
			Flags: "nzcv", FlagsWidth: 32, Arguments: args, Return: "x0"}
	default:
		return nil
	}
}

func isGoTableSection(name string) bool {
	return name == ".gopclntab" || name == ".data.rel.ro.gopclntab" || strings.HasSuffix(name, ",__gopclntab")
}

// RecoveryEvidence reports the size of the recovered function table.
func (Golang) RecoveryEvidence(count int) core.LanguageEvidence {
	return core.LanguageEvidence{Source: symbol.SourcePCLNTab, Detail: fmt.Sprintf("%d functions", count)}
}

func (Golang) Recover(a *core.Artifact) ([]symbol.Symbol, error) {
	if a.Reader == nil {
		return nil, fmt.Errorf("no reader for Go metadata")
	}
	// Mach-O uses a qualified section name; PE puts the same table in .rdata.
	// An external ELF linker may merge it into .data.rel.ro. Prefer the named
	// section so a byte pattern in unrelated data never outranks it.
	var candidates []core.Section
	for _, s := range a.Image.Sections {
		if isGoTableSection(s.Name) {
			candidates = append(candidates, s)
		}
	}
	if len(candidates) == 0 {
		for _, s := range a.Image.Sections {
			if s.Name == ".rdata" || s.Name == ".data.rel.ro" {
				candidates = append(candidates, s)
			}
		}
	}
	var lastErr error
	for _, s := range candidates {
		limit := metadataLimit(s.Name, a.Format)
		if s.FileSize == 0 || s.FileSize > limit {
			lastErr = fmt.Errorf("section %s is empty or exceeds %d bytes", s.Name, limit)
			continue
		}
		data, err := a.Reader.Bytes(s.FileOffset, s.FileSize)
		if err != nil {
			lastErr = err
			continue
		}
		for offset := 0; offset+16 <= len(data); offset += 4 {
			magic, order, ok := pclnHeader(data[offset:])
			if !ok {
				continue
			}
			syms, err := parseGoTable(a, data[offset:], s.VirtualAddress+uint64(offset), magic, order)
			if err == nil {
				return syms, nil
			}
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no supported Go 1.2+ pclntab found")
}

func pclnHeader(data []byte) (uint32, binary.ByteOrder, bool) {
	if len(data) < 16 || data[4] != 0 || data[5] != 0 ||
		(data[6] != 1 && data[6] != 2 && data[6] != 4) || (data[7] != 4 && data[7] != 8) {
		return 0, nil, false
	}
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		magic := order.Uint32(data)
		switch magic {
		case 0xfffffffb, 0xfffffffa, 0xfffffff0, 0xfffffff1:
			return magic, order, true
		}
	}
	return 0, nil, false
}

func parseGoTable(a *core.Artifact, data []byte, headerAddr uint64, magic uint32, order binary.ByteOrder) (symbols []symbol.Symbol, err error) {
	// debug/gosym handles most malformed tables itself, but its contract does
	// not promise panic-free parsing of untrusted compiler metadata.
	defer func() {
		if r := recover(); r != nil {
			symbols, err = nil, fmt.Errorf("invalid Go pclntab: %v", r)
		}
	}()
	ptr := int(data[7])
	word := func(offset int) uint64 {
		if ptr == 4 {
			return uint64(order.Uint32(data[offset:]))
		}
		return order.Uint64(data[offset:])
	}
	count := word(8)
	// Every function needs at least two 32-bit table entries. Check the count
	// before gosym allocates one Func and Sym per claimed entry.
	if count == 0 || count > uint64(len(data)/8) {
		return nil, fmt.Errorf("invalid pclntab function count %d", count)
	}
	// gosym reads entry offsets relative to the text base it is given. A Go
	// 1.18+ table stores offsets, not the base itself, so the base has to come
	// from runtime.text or runtime.moduledata. Using the start of the first
	// executable section is wrong when an external linker put C code there.
	text := uint64(0)
	if magic == 0xfffffff0 || magic == 0xfffffff1 {
		if len(data) < 8+8*ptr {
			return nil, fmt.Errorf("truncated pclntab header")
		}
		tables := readPclnTables(headerAddr, count, func(index int) uint64 { return word(8 + index*ptr) })
		base, err := textStart(a, tables, order)
		if err != nil {
			return nil, err
		}
		text = base
	} else if s, ok := a.Image.SymbolByName("runtime.text"); ok {
		text = s.Address
	} else if a.Hints != nil && len(a.Hints.ExecSections) != 0 {
		text = a.Hints.ExecSections[0].Address
	}
	table, err := gosym.NewTable(nil, gosym.NewLineTable(data, text))
	if err != nil {
		return nil, fmt.Errorf("decode pclntab: %w", err)
	}
	if uint64(len(table.Funcs)) != count {
		return nil, fmt.Errorf("pclntab decoded %d of %d functions", len(table.Funcs), count)
	}
	symbols = make([]symbol.Symbol, 0, len(table.Funcs))
	for _, f := range table.Funcs {
		if f.Sym == nil || f.Name == "" || f.End <= f.Entry || !executableRange(a, f.Entry, f.End-f.Entry) || bytes.IndexByte([]byte(f.Name), 0) >= 0 {
			return nil, fmt.Errorf("invalid pclntab function at 0x%x", f.Entry)
		}
		convention := symbol.Convention("")
		if strings.HasSuffix(f.Name, ".abi0") {
			// ABI0 wrappers use the platform convention even though pclntab
			// names them. Leave them out of the language ABI override.
			convention = symbol.ConventionPlatform
		}
		instance := ParseInstance(f.Name)
		name := instance.Name
		if name == "" {
			name = f.Name
		}
		entry := symbol.Symbol{
			Name:       name,
			Address:    f.Entry,
			Size:       goFuncSize(a, f.Entry, f.End-f.Entry, a.Target.Architecture),
			Kind:       symbol.SymbolFunction,
			Source:     symbol.SourcePCLNTab,
			Convention: convention,
		}
		if name != f.Name {
			// Keep the exact pclntab spelling for lookups and for consumers
			// that need the compiler's go.shape. type arguments.
			entry.Mangled = f.Name
		}
		if instance.Generic || instance.Synthetic || instance.Receiver != "" || instance.Closure != "" {
			entry.Identity = instance.Identity()
		}
		symbols = append(symbols, entry)
	}
	return symbols, nil
}

// pclntab ends a function at the next entry, so the recorded range also holds
// the linker's alignment fill. Decoding that fill turns padding into
// instructions. pclntab has no exact size, so only a bounded tail is trimmed.
const maxGoPadding = 64

func goFuncSize(a *core.Artifact, address, size uint64, arch core.Architecture) uint64 {
	window := size
	if window > maxGoPadding {
		window = maxGoPadding
	}
	if window == 0 {
		return size
	}
	region, err := a.RegionAt(address+size-window, window)
	if err != nil {
		return size
	}
	reader, err := a.RegionReader(region)
	if err != nil {
		return size
	}
	data, err := reader.Bytes(0, window)
	if err != nil {
		return size
	}
	switch arch {
	case core.ArchAMD64, core.Arch386, core.Arch8086:
		for size > 1 && len(data) > 0 {
			last := data[len(data)-1]
			if last != 0xcc && last != 0x90 {
				break
			}
			data, size = data[:len(data)-1], size-1
		}
	case core.ArchARM64:
		for size > 4 && len(data) >= 4 {
			word := binary.LittleEndian.Uint32(data[len(data)-4:])
			// 0xd503201f is nop; zero words are the linker's fill.
			if word != 0xd503201f && word != 0 {
				break
			}
			data, size = data[:len(data)-4], size-4
		}
	}
	return size
}

func executableRange(a *core.Artifact, address, size uint64) bool {
	if a.Hints == nil {
		return false
	}
	for _, r := range a.Hints.ExecSections {
		if address >= r.Address && address-r.Address < r.Size && size <= r.Size-(address-r.Address) {
			return true
		}
	}
	return false
}
