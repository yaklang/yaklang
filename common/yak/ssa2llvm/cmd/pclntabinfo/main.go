// pclntabinfo breaks a Go binary's .gopclntab into its sub-tables and reports
// the size of each, so the "symbolization data" (names/files/lines) can be told
// apart from the runtime-critical data (ftab/_func/pcsp/funcdata).
//
// Layout (Go 1.18+, see runtime/symtab.go pcHeader):
//
//	header | funcnametab | cutab | filetab | pctab | pclntab(ftab + _func + funcdata)
package main

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
)

type pcHeader struct {
	magic          uint32
	minLC          uint8
	ptrSize        uint8
	nfunc          uint64
	nfiles         uint64
	textStart      uint64
	funcnameOffset uint64
	cuOffset       uint64
	filetabOffset  uint64
	pctabOffset    uint64
	pclnOffset     uint64
}

func parseHeader(b []byte) (pcHeader, error) {
	var h pcHeader
	if len(b) < 64 {
		return h, fmt.Errorf("pclntab too short")
	}
	h.magic = binary.LittleEndian.Uint32(b[0:4])
	switch h.magic {
	case 0xFFFFFFF0, 0xFFFFFFF1: // go1.18 / go1.20+
	default:
		return h, fmt.Errorf("unrecognized pclntab magic %#x", h.magic)
	}
	h.minLC = b[6]
	h.ptrSize = b[7]
	u := func(i int) uint64 { return binary.LittleEndian.Uint64(b[i:]) }
	h.nfunc = u(8)
	h.nfiles = u(16)
	h.textStart = u(24)
	h.funcnameOffset = u(32)
	h.cuOffset = u(40)
	h.filetabOffset = u(48)
	h.pctabOffset = u(56)
	h.pclnOffset = u(64)
	return h, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pclntabinfo <binary> [...]")
		os.Exit(2)
	}
	for _, path := range os.Args[1:] {
		if err := report(path); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
		}
		fmt.Println()
	}
}

func report(path string) error {
	f, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sec := f.Section(".gopclntab")
	if sec == nil {
		return fmt.Errorf("no .gopclntab section")
	}
	data, err := sec.Data()
	if err != nil {
		return err
	}
	h, err := parseHeader(data)
	if err != nil {
		return err
	}
	total := uint64(len(data))

	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	binSize := uint64(fi.Size())

	type region struct {
		name     string
		size     uint64
		critical bool
		note     string
	}
	ftabSize := h.nfunc * 8 // functab{entryoff, funcoff} + sentinel handled below
	regions := []region{
		{"header", h.funcnameOffset, true, "pcHeader"},
		{"funcnametab", h.cuOffset - h.funcnameOffset, false, "函数名字符串"},
		{"cutab", h.filetabOffset - h.cuOffset, false, "编译单元→文件索引"},
		{"filetab", h.pctabOffset - h.filetabOffset, false, "源文件名字符串"},
		{"pctab", h.pclnOffset - h.pctabOffset, true, "pc-value 变长表（pcsp/pcfile/pcln/pcdata 混合）"},
		{"ftab+_func+funcdata", total - h.pclnOffset, true, "函数表 + _func 结构 + GC stackmap 指针"},
	}

	fmt.Printf("== %s ==\n", path)
	fmt.Printf("binary            : %d bytes (%.2f MiB)\n", binSize, mib(binSize))
	fmt.Printf(".gopclntab        : %d bytes (%.2f MiB, %.1f%% of binary)\n", total, mib(total), 100*float64(total)/float64(binSize))
	fmt.Printf("nfunc / nfiles    : %d / %d\n", h.nfunc, h.nfiles)
	fmt.Printf("ftab entries      : %d (= %d bytes of the last region)\n\n", h.nfunc+1, ftabSize+8)

	var symbolization, critical uint64
	for _, r := range regions {
		kind := "运行时必需"
		if !r.critical {
			kind = "仅符号化"
			symbolization += r.size
		} else {
			critical += r.size
		}
		fmt.Printf("  %-22s %12d  %7.2f MiB  %5.1f%%  %-10s %s\n",
			r.name, r.size, mib(r.size), 100*float64(r.size)/float64(total), kind, r.note)
	}
	fmt.Printf("\n  %-22s %12d  %7.2f MiB  %5.1f%% of pclntab  (= %.1f%% of binary)\n",
		"仅符号化 小计", symbolization, mib(symbolization),
		100*float64(symbolization)/float64(total), 100*float64(symbolization)/float64(binSize))
	fmt.Printf("  %-22s %12d  %7.2f MiB  %5.1f%% of pclntab\n",
		"运行时必需 小计", critical, mib(critical), 100*float64(critical)/float64(total))

	reportTopPackages(data, h)
	return nil
}

// reportTopPackages buckets every function in the ftab by package path, so the
// biggest contributors to the function count (and therefore to pclntab) show up.
func reportTopPackages(data []byte, h pcHeader) {
	names := data[h.funcnameOffset:h.cuOffset]
	ftab := data[h.pclnOffset:]

	counts := map[string]int{}
	nameBytes := map[string]int{}
	shown := 0
	for i := uint64(0); i < h.nfunc; i++ {
		base := i * 8
		if base+8 > uint64(len(ftab)) {
			break
		}
		funcoff := uint64(binary.LittleEndian.Uint32(ftab[base+4:]))
		if funcoff+8 > uint64(len(ftab)) {
			continue
		}
		nameOff := int32(binary.LittleEndian.Uint32(ftab[funcoff+4:]))
		if nameOff < 0 || uint64(nameOff) >= uint64(len(names)) {
			continue
		}
		name := cstr(names[nameOff:])
		if shown < 3 {
			fmt.Printf("    (sample func: %s)\n", name)
			shown++
		}
		pkg := pkgOf(name)
		counts[pkg]++
		nameBytes[pkg] += len(name) + 1
	}

	list := make([]pkgStat, 0, len(counts))
	for p, n := range counts {
		list = append(list, pkgStat{p, n, nameBytes[p]})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n > list[j].n })

	fmt.Printf("\n  函数数量 Top 15 包前缀（共 %d 个函数）：\n", h.nfunc)
	for i, e := range list {
		if i >= 15 {
			break
		}
		fmt.Printf("    %-52s %7d 个  %5.1f%%  名字表 %8d B\n",
			e.pkg, e.n, 100*float64(e.n)/float64(h.nfunc), e.bytes)
	}
}

type pkgStat struct {
	pkg   string
	n     int
	bytes int
}

func cstr(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// pkgOf reduces a Go symbol name to a coarse package bucket.
func pkgOf(name string) string {
	// strip method receiver / generic decorations after the last path segment
	slash := -1
	for i := 0; i < len(name); i++ {
		if name[i] == '/' {
			slash = i
		}
	}
	seg := name
	if slash >= 0 {
		seg = name[slash+1:]
	}
	dot := -1
	for i := 0; i < len(seg); i++ {
		if seg[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		if slash < 0 {
			return name
		}
		return name[:slash]
	}
	if slash < 0 {
		return seg[:dot]
	}
	return name[:slash+1] + seg[:dot]
}

func mib(v uint64) float64 { return float64(v) / (1 << 20) }
