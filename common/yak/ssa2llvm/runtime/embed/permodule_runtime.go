package embed

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// WriteRuntimeImportsPerModule generates a runtime_imports_generated.go that
// imports the given yaklib modules but does NOT blanket-register them in init().
// Instead it emits one C-exported `yak_register_module_<m>()` function per
// module, each calling runtimeRegisterYaklibModule("m", m.ExportExpr).
//
// The compiler then emits calls to yak_register_module_<used>() for the modules
// a script actually uses, so lld --gc-sections can drop the unused modules'
// code from a single (full) libyak.a — giving per-script minimal output without
// a Go toolchain at runtime. This is the C' approach; validate before adopting.
//
// If modules contains "all", every registered module is used. Modules with no
// prunedExportSources are skipped; unknown names are ignored.
func WriteRuntimeImportsPerModule(outputPath string, modules []string) error {
	want := map[string]bool{}
	for _, m := range modules {
		m = strings.TrimSpace(m)
		if m != "" {
			want[m] = true
		}
	}
	useAll := want["all"]

	type imp struct{ alias, path string }
	type reg struct{ module, exportExpr string }

	impSeen := map[string]bool{}
	var imports []imp
	var regs []reg
	for _, name := range AllModuleNames() {
		if !useAll && !want[name] {
			continue
		}
		spec, ok := LookupModuleSpec(name)
		if !ok || len(spec.prunedExportSources()) == 0 {
			continue
		}
		for _, src := range spec.prunedExportSources() {
			if !impSeen[src.ImportAlias] {
				impSeen[src.ImportAlias] = true
				imports = append(imports, imp{src.ImportAlias, src.GoImportPath})
			}
			regs = append(regs, reg{name, src.ExportExpr})
		}
	}

	byMod := map[string][]string{}
	var modNames []string
	for _, r := range regs {
		if _, ok := byMod[r.module]; !ok {
			modNames = append(modNames, r.module)
		}
		byMod[r.module] = append(byMod[r.module], r.exportExpr)
	}
	sort.Strings(modNames)

	var b strings.Builder
	b.WriteString("package main\n\n")
	// //export requires import "C" (cgo). Minimal preamble.
	b.WriteString("import \"C\"\n\n")
	if len(imports) > 0 {
		b.WriteString("import (\n")
		sorted := make([]imp, len(imports))
		copy(sorted, imports)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].alias < sorted[j].alias })
		for _, im := range sorted {
			b.WriteString(fmt.Sprintf("\t%s %q\n", im.alias, im.path))
		}
		b.WriteString(")\n\n")
	}
	// Empty init: NO blanket registration (so --gc-sections can drop unused modules).
	b.WriteString("func init() {}\n\n")
	for _, m := range modNames {
		b.WriteString(fmt.Sprintf("//export yak_register_module_%s\n", m))
		b.WriteString(fmt.Sprintf("func yak_register_module_%s() {\n", m))
		for _, expr := range byMod[m] {
			b.WriteString(fmt.Sprintf("\truntimeRegisterYaklibModule(%q, %s)\n", m, expr))
		}
		b.WriteString("}\n\n")
	}
	return os.WriteFile(outputPath, []byte(b.String()), 0o644)
}