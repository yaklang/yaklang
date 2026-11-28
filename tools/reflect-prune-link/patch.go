package main

import (
	"fmt"
	"strings"
)

const (
	deadcodeImportAnchor = "\t\"strings\"\n"
	deadcodeLoopAnchor   = "\t\t\tif (d.reflectSeen && (m.isExported() || d.dynlink)) || d.ifaceMethod[m.m] || d.genericIfaceMethod[m.m.name] {"
	deadcodeAfterLoop    = "\t\td.flood()\n\t}\n"
)

const deadcodeHelperBlock = `
type yakReflectPruneFilter struct {
	all      bool
	excludes []string
	allows   []string
	verbose  bool
}

func loadYakReflectPruneFilter() *yakReflectPruneFilter {
	f := &yakReflectPruneFilter{
		verbose: os.Getenv("YAK_REFLECT_PRUNE_VERBOSE") == "1",
	}
	var rawParts []string
	if v := strings.TrimSpace(os.Getenv("YAK_REFLECT_PRUNE_EXCLUDE")); v != "" {
		rawParts = append(rawParts, v)
	}
	if fp := strings.TrimSpace(os.Getenv("YAK_REFLECT_PRUNE_EXCLUDE_FILE")); fp != "" {
		if data, err := os.ReadFile(fp); err == nil {
			rawParts = append(rawParts, string(data))
		}
	}
	for _, raw := range rawParts {
		for _, line := range strings.Split(raw, "\n") {
			if idx := strings.IndexByte(line, '#'); idx >= 0 {
				line = line[:idx]
			}
			for _, tok := range strings.Split(line, ",") {
				tok = strings.TrimSpace(tok)
				if tok == "" {
					continue
				}
				if tok == "ALL" {
					f.all = true
					continue
				}
				if strings.HasPrefix(tok, "!") {
					if pat := strings.TrimSpace(tok[1:]); pat != "" {
						f.allows = append(f.allows, pat)
					}
					continue
				}
				f.excludes = append(f.excludes, tok)
			}
		}
	}
	if !f.all && len(f.excludes) == 0 {
		return nil
	}
	return f
}

func (f *yakReflectPruneFilter) match(ldr *loader.Loader, src loader.Sym) bool {
	if f == nil {
		return false
	}
	name := ldr.SymName(src)
	for _, a := range f.allows {
		if strings.Contains(name, a) {
			return false
		}
	}
	if f.all {
		return true
	}
	for _, p := range f.excludes {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}
`

// PatchDeadcodeSource injects selective reflectSeen method-pruning support
// into Go's cmd/link/internal/ld/deadcode.go without mutating GOROOT.
//
// It verifies exact anchor lines so a future Go toolchain upgrade fails loudly
// instead of silently producing an unpatched linker.
func PatchDeadcodeSource(orig []byte) ([]byte, error) {
	src := string(orig)

	if strings.Contains(src, "loadYakReflectPruneFilter") {
		return orig, nil
	}

	if !strings.Contains(src, deadcodeLoopAnchor) {
		return nil, fmt.Errorf("deadcode.go anchor not found: %q (Go toolchain version may have changed)", strings.TrimSpace(deadcodeLoopAnchor))
	}
	if !strings.Contains(src, deadcodeAfterLoop) {
		return nil, fmt.Errorf("deadcode.go post-loop anchor not found (Go toolchain version may have changed)")
	}

	// Ensure "os" is imported.
	if !strings.Contains(src, "\"os\"") {
		if !strings.Contains(src, deadcodeImportAnchor) {
			return nil, fmt.Errorf("deadcode.go import anchor not found")
		}
		src = strings.Replace(src, deadcodeImportAnchor, "\t\"os\"\n"+deadcodeImportAnchor, 1)
	}

	// Initialize filter right before the method-marking loop.
	const forLoopStart = "\tfor {\n\t\t// Mark all methods that could satisfy a discovered"
	if !strings.Contains(src, forLoopStart) {
		return nil, fmt.Errorf("deadcode.go for-loop anchor not found")
	}
	src = strings.Replace(
		src,
		forLoopStart,
		"\tyakReflectFilter := loadYakReflectPruneFilter()\n"+forLoopStart,
		1,
	)

	// Gate reflectSeen retention on !yakReflectFilter.match(d.ldr, m.src).
	// Direct calls and interface-satisfying methods remain 100% untouched.
	patchedCond := "\t\t\tif (d.reflectSeen && (m.isExported() || d.dynlink) && !yakReflectFilter.match(d.ldr, m.src)) || d.ifaceMethod[m.m] || d.genericIfaceMethod[m.m.name] {"
	src = strings.Replace(src, deadcodeLoopAnchor, patchedCond, 1)

	// Optional verbose diagnostic after the loop finishes.
	const verboseReport = "\t\td.flood()\n\t}\n" +
		"\tif yakReflectFilter != nil && yakReflectFilter.verbose && d.reflectSeen {\n" +
		"\t\tpruned := 0\n" +
		"\t\tfor _, m := range d.markableMethods {\n" +
		"\t\t\tif (m.isExported() || d.dynlink) && yakReflectFilter.match(d.ldr, m.src) {\n" +
		"\t\t\t\tpruned++\n" +
		"\t\t\t}\n" +
		"\t\t}\n" +
		"\t\tfmt.Fprintf(os.Stderr, \"[reflect-prune] pruned %d reflect-only exported methods (all=%v excludes=%d allows=%d)\\n\", pruned, yakReflectFilter.all, len(yakReflectFilter.excludes), len(yakReflectFilter.allows))\n" +
		"\t}\n"
	src = strings.Replace(src, deadcodeAfterLoop, verboseReport, 1)

	src += deadcodeHelperBlock
	return []byte(src), nil
}
