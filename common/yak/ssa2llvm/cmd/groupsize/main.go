// groupsize buckets the functions of a .modtext.<group> section by package, so
// a split of that group can be decided from where the bytes actually are.
//
// Usage: groupsize <object> <section> [depth]
package main

import (
	"debug/elf"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: groupsize <object> <section> [depth]")
		os.Exit(2)
	}
	depth := 4
	if len(os.Args) > 3 {
		depth, _ = strconv.Atoi(os.Args[3])
	}
	f, err := elf.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	target := -1
	for i, s := range f.Sections {
		if s.Name == os.Args[2] {
			target = i
			break
		}
	}
	if target < 0 {
		fmt.Fprintf(os.Stderr, "section %q not found\n", os.Args[2])
		os.Exit(1)
	}
	syms, err := f.Symbols()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	type stat struct {
		bytes uint64
		funcs int
	}
	byPkg := map[string]*stat{}
	var total uint64
	for _, s := range syms {
		if int(s.Section) != target || elf.ST_TYPE(s.Info) != elf.STT_FUNC {
			continue
		}
		pkg := packagePrefix(s.Name, depth)
		st := byPkg[pkg]
		if st == nil {
			st = &stat{}
			byPkg[pkg] = st
		}
		st.bytes += s.Size
		st.funcs++
		total += s.Size
	}

	pkgs := make([]string, 0, len(byPkg))
	for p := range byPkg {
		pkgs = append(pkgs, p)
	}
	// GROUPSIZE_PACKAGES prints just the package names, for use as the
	// allowed set when splitting a group: only packages already in it may
	// move, or base code loses something it calls.
	if os.Getenv("GROUPSIZE_PACKAGES") != "" {
		sort.Strings(pkgs)
		for _, p := range pkgs {
			fmt.Println(p)
		}
		return
	}
	sort.Slice(pkgs, func(i, j int) bool { return byPkg[pkgs[i]].bytes > byPkg[pkgs[j]].bytes })
	fmt.Printf("%s: %.2f MiB over %d packages\n", os.Args[2], float64(total)/(1<<20), len(pkgs))
	for _, p := range pkgs {
		st := byPkg[p]
		if st.bytes < 64*1024 {
			continue
		}
		fmt.Printf("  %8.2f MiB  %6d funcs  %s\n", float64(st.bytes)/(1<<20), st.funcs, p)
	}
}

// packagePrefix trims a Go symbol down to its package path at the requested
// path depth, so sibling packages under one dependency collapse into one row.
func packagePrefix(sym string, depth int) string {
	if i := strings.LastIndex(sym, "/"); i >= 0 {
		if j := strings.IndexAny(sym[i:], ".("); j >= 0 {
			sym = sym[:i+j]
		}
	} else if j := strings.IndexAny(sym, ".("); j >= 0 {
		sym = sym[:j]
	}
	parts := strings.Split(sym, "/")
	if len(parts) > depth {
		parts = parts[:depth]
	}
	return strings.Join(parts, "/")
}
