// pclnrefs lists every relocation in a Go c-archive object that targets the
// runtime.pclntab symbol, grouped by the section holding the relocation.
//
// Folding a sub-table out of .gopclntab shifts every byte after it, so each of
// these addends has to be adjusted. Any group other than .rela.go.module is a
// reference this pass does not know how to fix.
package main

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
)

func main() {
	f, err := elf.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	syms, err := f.Symbols()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	target := -1
	for i, s := range syms {
		if s.Name == "runtime.pclntab" {
			target = i + 1 // raw index: debug/elf drops the null symbol
			break
		}
	}
	if target < 0 {
		fmt.Fprintln(os.Stderr, "runtime.pclntab not found")
		os.Exit(1)
	}

	type group struct {
		count   int
		addends []uint64
	}
	groups := map[string]*group{}
	for _, sec := range f.Sections {
		if sec.Type != elf.SHT_RELA {
			continue
		}
		data, err := sec.Data()
		if err != nil {
			continue
		}
		for off := 0; off+24 <= len(data); off += 24 {
			if int(binary.LittleEndian.Uint64(data[off+8:])>>32) != target {
				continue
			}
			g := groups[sec.Name]
			if g == nil {
				g = &group{}
				groups[sec.Name] = g
			}
			g.count++
			if len(g.addends) < 12 {
				g.addends = append(g.addends, binary.LittleEndian.Uint64(data[off+16:]))
			}
		}
	}

	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		g := groups[name]
		fmt.Printf("%-24s %6d relocs, first addends: %v\n", name, g.count, g.addends)
	}
}
