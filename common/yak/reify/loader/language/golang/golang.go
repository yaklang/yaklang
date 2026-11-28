package golang

import (
	"bytes"
	"debug/buildinfo"
	"debug/gosym"
	"encoding/binary"
	"fmt"
	"math"
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
	maxFuncNameTabSize  = 128 << 20
	pclnScanChunkSize   = 4 << 20
	pclnWindowSize      = 1 << 20
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
	var (
		lastErr error
		scanBuf []byte
	)
	const overlap = 64
	for _, s := range candidates {
		limit := metadataLimit(s.Name, a.Format)
		if s.FileSize == 0 || s.FileSize > limit {
			lastErr = fmt.Errorf("section %s is empty or exceeds %d bytes", s.Name, limit)
			continue
		}
		chunkSize := s.FileSize
		if chunkSize > pclnScanChunkSize {
			chunkSize = pclnScanChunkSize
		}
		if cap(scanBuf) < int(chunkSize) {
			scanBuf = make([]byte, int(chunkSize))
		}
		buf := scanBuf[:int(chunkSize)]
		for pos := uint64(0); pos < s.FileSize; {
			avail := s.FileSize - pos
			if avail < 16 {
				break
			}
			readLen := avail
			if readLen > chunkSize {
				readLen = chunkSize
			}
			n, err := a.Reader.ReadAt(buf[:int(readLen)], int64(s.FileOffset+pos))
			if n < 16 || (err != nil && n == 0) {
				if err != nil {
					lastErr = err
				}
				break
			}
			chunk := buf[:n]
			for offset := 0; offset+16 <= len(chunk); offset += 4 {
				magic, order, ok := pclnHeader(chunk[offset:])
				if !ok {
					continue
				}
				relOffset := pos + uint64(offset)
				var syms []symbol.Symbol
				var parseErr error
				if magic == 0xfffffff0 || magic == 0xfffffff1 {
					syms, parseErr = parseGo118TableDirect(a, s, relOffset, order)
				} else {
					data, readErr := a.Reader.Bytes(s.FileOffset+relOffset, s.FileSize-relOffset)
					if readErr != nil {
						lastErr = readErr
						continue
					}
					syms, parseErr = parseGoTable(a, data, s.VirtualAddress+relOffset, magic, order)
				}
				if parseErr == nil {
					return syms, nil
				}
				lastErr = parseErr
			}
			if pos+uint64(n) >= s.FileSize || uint64(n) <= overlap {
				break
			}
			advance := uint64(n) - overlap
			advance &^= 3
			if advance == 0 {
				advance = 4
			}
			pos += advance
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
	// Go 1.18+ entries are offsets from runtime.text, not from the start of
	// the container's text section. An external linker can put C code first.
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

// slidingWindow reads sequential slices from ByteReader using a single bounded buffer.
type slidingWindow struct {
	reader   *core.ByteReader
	base     uint64
	limit    uint64
	winStart uint64
	validLen uint64
	buf      []byte
}

func newSlidingWindow(reader *core.ByteReader, base, limit uint64, capBytes int) *slidingWindow {
	return &slidingWindow{
		reader: reader,
		base:   base,
		limit:  limit,
		buf:    make([]byte, capBytes),
	}
}

func (w *slidingWindow) view(fileOff, need uint64) ([]byte, bool) {
	if w == nil || w.reader == nil || need == 0 || fileOff < w.base || fileOff > w.limit || need > w.limit-fileOff || need > uint64(len(w.buf)) {
		return nil, false
	}
	if fileOff >= w.winStart && fileOff-w.winStart+need <= w.validLen {
		start := int(fileOff - w.winStart)
		return w.buf[start : start+int(need)], true
	}
	avail := w.limit - fileOff
	if avail > uint64(len(w.buf)) {
		avail = uint64(len(w.buf))
	}
	n, err := w.reader.ReadAt(w.buf[:int(avail)], int64(fileOff))
	if uint64(n) < need || (err != nil && n == 0) {
		w.validLen = 0
		return nil, false
	}
	w.winStart = fileOff
	w.validLen = uint64(n)
	return w.buf[:int(need)], true
}

// parseGo118TableDirect decodes Go 1.18+ (0xfffffff0/0xfffffff1) pclntab headers
// without reading the whole section or instantiating debug/gosym.Table's pctab,
// filetab, or Func/Sym wrapper arrays.
func parseGo118TableDirect(a *core.Artifact, s core.Section, relOffset uint64, order binary.ByteOrder) ([]symbol.Symbol, error) {
	headerAddr := s.VirtualAddress + relOffset
	headerFileOff := s.FileOffset + relOffset
	sectionRemain := s.FileSize - relOffset
	if sectionRemain < 16 {
		return nil, fmt.Errorf("truncated pclntab header")
	}
	headLen := uint64(72)
	if sectionRemain < headLen {
		headLen = sectionRemain
	}
	head, err := a.Reader.Bytes(headerFileOff, headLen)
	if err != nil {
		return nil, err
	}
	ptr := int(head[7])
	if len(head) < 8+8*ptr {
		return nil, fmt.Errorf("truncated pclntab header")
	}
	word := func(slot int) uint64 {
		off := 8 + slot*ptr
		if ptr == 4 {
			return uint64(order.Uint32(head[off:]))
		}
		return order.Uint64(head[off:])
	}
	count := word(0)
	if count == 0 || count > sectionRemain/8 {
		return nil, fmt.Errorf("invalid pclntab function count %d", count)
	}
	tables := readPclnTables(headerAddr, count, func(index int) uint64 { return word(index) })
	if tables.pclnTable >= sectionRemain || tables.funcNames >= sectionRemain {
		return nil, fmt.Errorf("invalid pclntab subtable offset")
	}
	functabBytes := (count*2 + 1) * 4
	if functabBytes > sectionRemain-tables.pclnTable {
		return nil, fmt.Errorf("truncated pclntab function table")
	}
	funcNameEnd := sectionRemain
	for _, cand := range []uint64{tables.cuTab, tables.fileTab, tables.pcTab, tables.pclnTable} {
		if cand > tables.funcNames && cand < funcNameEnd {
			funcNameEnd = cand
		}
	}
	funcNameSize := funcNameEnd - tables.funcNames
	if funcNameSize == 0 || funcNameSize > maxFuncNameTabSize {
		return nil, fmt.Errorf("invalid pclntab funcnametab size %d", funcNameSize)
	}
	text, err := textStart(a, tables, order)
	if err != nil {
		return nil, err
	}
	funcnametab, err := a.Reader.Bytes(headerFileOff+tables.funcNames, funcNameSize)
	if err != nil {
		return nil, fmt.Errorf("read pclntab funcnametab: %w", err)
	}
	// Copy funcnametab once into an immutable string pool so every function name
	// and ParseInstance substring slices a single contiguous allocation instead of
	// creating ~200K small heap strings.
	namePool := string(funcnametab)
	funcnametab = nil

	functab, err := a.Reader.Bytes(headerFileOff+tables.pclnTable, functabBytes)
	if err != nil {
		return nil, fmt.Errorf("read pclntab functab: %w", err)
	}
	funcDataBase := headerFileOff + tables.pclnTable
	funcDataLimit := s.FileOffset + s.FileSize
	funcDataWin := newSlidingWindow(a.Reader, funcDataBase, funcDataLimit, pclnWindowSize)
	var execWins []*slidingWindow
	if a.Hints != nil && len(a.Hints.ExecSections) > 0 {
		execWins = make([]*slidingWindow, len(a.Hints.ExecSections))
	}

	symbols := make([]symbol.Symbol, 0, count)
	for i := uint64(0); i < count; i++ {
		entryOff := uint64(order.Uint32(functab[i*8:]))
		funcInfoOff := uint64(order.Uint32(functab[i*8+4:]))
		nextEntryOff := uint64(order.Uint32(functab[(i+1)*8:]))
		if nextEntryOff <= entryOff || text > math.MaxUint64-nextEntryOff {
			return nil, fmt.Errorf("invalid pclntab function range at index %d", i)
		}
		entry := text + entryOff
		rawSize := nextEntryOff - entryOff
		regionIdx := executableRegionIndex(a, entry, rawSize)
		if regionIdx < 0 {
			return nil, fmt.Errorf("invalid pclntab function at 0x%x", entry)
		}
		info, ok := funcDataWin.view(funcDataBase+funcInfoOff, 8)
		if !ok {
			return nil, fmt.Errorf("truncated pclntab funcdata at 0x%x", entry)
		}
		nameOff := order.Uint32(info[4:8])
		if uint64(nameOff) >= uint64(len(namePool)) {
			return nil, fmt.Errorf("invalid pclntab name offset at 0x%x", entry)
		}
		nul := strings.IndexByte(namePool[nameOff:], 0)
		if nul <= 0 {
			return nil, fmt.Errorf("invalid pclntab function at 0x%x", entry)
		}
		rawName := namePool[nameOff : uint64(nameOff)+uint64(nul)]
		var execWin *slidingWindow
		if execWins != nil {
			if execWins[regionIdx] == nil {
				r := a.Hints.ExecSections[regionIdx]
				execWins[regionIdx] = newSlidingWindow(a.Reader, r.Offset, r.Offset+r.Size, pclnWindowSize)
			}
			execWin = execWins[regionIdx]
		}
		trimmedSize := goFuncSizeWithWindow(a, regionIdx, execWin, entry, rawSize, a.Target.Architecture)
		symbols = append(symbols, buildGoSymbol(rawName, entry, trimmedSize))
	}
	return symbols, nil
}

func buildGoSymbol(rawName string, entryAddr, size uint64) symbol.Symbol {
	convention := symbol.Convention("")
	if strings.HasSuffix(rawName, ".abi0") {
		// ABI0 wrappers use the platform convention even though pclntab
		// names them. Leave them out of the language ABI override.
		convention = symbol.ConventionPlatform
	}
	instance := ParseInstance(rawName)
	name := instance.Name
	if name == "" {
		name = rawName
	}
	entry := symbol.Symbol{
		Name:       name,
		Address:    entryAddr,
		Size:       size,
		Kind:       symbol.SymbolFunction,
		Source:     symbol.SourcePCLNTab,
		Convention: convention,
	}
	if name != rawName {
		// Keep the exact pclntab spelling for lookups and for consumers
		// that need the compiler's go.shape. type arguments.
		entry.Mangled = rawName
	}
	if instance.Generic || instance.Synthetic || instance.Receiver != "" || instance.Closure != "" {
		entry.Identity = instance.Identity()
	}
	return entry
}

func executableRegionIndex(a *core.Artifact, address, size uint64) int {
	if a.Hints == nil {
		return -1
	}
	for idx, r := range a.Hints.ExecSections {
		if address >= r.Address && address-r.Address < r.Size && size <= r.Size-(address-r.Address) {
			return idx
		}
	}
	return -1
}

func goFuncSizeWithWindow(a *core.Artifact, regionIdx int, win *slidingWindow, address, size uint64, arch core.Architecture) uint64 {
	window := size
	if window > maxGoPadding {
		window = maxGoPadding
	}
	if window == 0 {
		return size
	}
	if win != nil && a.Hints != nil && regionIdx >= 0 && regionIdx < len(a.Hints.ExecSections) {
		r := a.Hints.ExecSections[regionIdx]
		fileOff := r.Offset + (address + size - window - r.Address)
		if data, ok := win.view(fileOff, window); ok {
			return trimGoFuncPadding(data, size, arch)
		}
	}
	return goFuncSize(a, address, size, arch)
}

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
	return trimGoFuncPadding(data, size, arch)
}

func trimGoFuncPadding(data []byte, size uint64, arch core.Architecture) uint64 {
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
