// linkbench links one already-compiled script object multiple times, varying
// only the retained yaklib module set, so the size/time cost of link-time
// pruning can be measured as a controlled A/B (same object, same archive, same
// linker; only UsedModules differs).
//
// Usage: linkbench <script-object.o> <out-dir> <name=mod,mod,...> [...]
//
//	linkbench out.o /tmp/lb 'pruned=' 'full=os,poc,cli,http,codec,yakit,ssa,shared,ssafront'
package main

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

type arm struct {
	name    string
	modules []string
}

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: linkbench <object.o> <out-dir> <name=mod,mod,...> [...]")
		os.Exit(2)
	}
	objFile := os.Args[1]
	outDir := os.Args[2]

	arms := make([]arm, 0, len(os.Args)-3)
	for _, spec := range os.Args[3:] {
		name, list, ok := strings.Cut(spec, "=")
		if !ok {
			fmt.Fprintf(os.Stderr, "bad arm spec %q, want name=mod,mod\n", spec)
			os.Exit(2)
		}
		var mods []string
		for _, m := range strings.Split(list, ",") {
			if m = strings.TrimSpace(m); m != "" {
				mods = append(mods, m)
			}
		}
		arms = append(arms, arm{name: name, modules: mods})
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", outDir, err)
		os.Exit(1)
	}

	fmt.Printf("object   : %s\n", objFile)
	fmt.Printf("out-dir  : %s\n\n", outDir)

	for _, a := range arms {
		workDir := filepath.Join(outDir, "work-"+a.name)
		binFile := filepath.Join(outDir, a.name+".bin")
		if err := os.RemoveAll(workDir); err != nil {
			fmt.Fprintf(os.Stderr, "clean workdir: %v\n", err)
			os.Exit(1)
		}
		if err := os.MkdirAll(workDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir workdir: %v\n", err)
			os.Exit(1)
		}

		start := time.Now()
		err := compiler.CompileObjectToBinarySCWithPatch(objFile, binFile, workDir, nil, a.modules)
		elapsed := time.Since(start)
		if err != nil {
			fmt.Printf("[%s] FAILED after %s: %v\n\n", a.name, elapsed.Round(time.Millisecond), err)
			continue
		}

		fi, err := os.Stat(binFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat %s: %v\n", binFile, err)
			os.Exit(1)
		}
		retained, total := modtextSections(binFile)

		fmt.Printf("[%s]\n", a.name)
		fmt.Printf("  requested modules : %v\n", a.modules)
		fmt.Printf("  link wall time    : %s\n", elapsed.Round(time.Millisecond))
		fmt.Printf("  binary size       : %d bytes (%.2f MiB)\n", fi.Size(), float64(fi.Size())/(1<<20))
		fmt.Printf("  retained .modtext : %v (%d bytes total)\n", sortedNames(retained), total)
		for _, name := range sortedNames(retained) {
			fmt.Printf("      .modtext.%-9s %10d bytes\n", name, retained[name])
		}
		fmt.Println()
	}
}

func modtextSections(path string) (map[string]uint64, uint64) {
	out := map[string]uint64{}
	var total uint64
	f, err := elf.Open(path)
	if err != nil {
		return out, 0
	}
	defer f.Close()
	for _, s := range f.Sections {
		if !strings.HasPrefix(s.Name, ".modtext.") {
			continue
		}
		name := strings.TrimPrefix(s.Name, ".modtext.")
		if name == "" {
			continue
		}
		out[name] = s.Size
		total += s.Size
	}
	return out, total
}

func sortedNames(m map[string]uint64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
