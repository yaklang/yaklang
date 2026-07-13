package embed

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// WriteRuntimeImports generates a runtime_imports_generated.go that imports
// and registers the given yaklib modules AND the full set of global callables
// (yaklib.GlobalExport + builtin.YaklangBaseLib + len/cap), so a libyak.a built
// from it has those modules' exports and every global builtin available at
// runtime.
//
// Used by scripts/build_runtime_embed.sh to build the embedded runtime for the
// self-contained ssa2llvm binary. The on-demand pruned build
// (BuildPrunedRuntimeArchiveFromLocalSourceWithDeps) generates a minimal runtime
// per script but needs the Go toolchain + source tree at runtime, which the
// self-contained (zero-dep) mode cannot assume; the embedded runtime is built
// once, at ssa2llvm build time, for a configured set of modules.
//
// If modules contains "all", every registered module is used. Modules whose
// prunedExportSources() is empty are skipped (no pruned-runtime export wiring
// yet) so generation does not error out. Unknown module names are ignored.
func WriteRuntimeImports(outputPath string, modules []string) error {
	want := map[string]bool{}
	for _, m := range modules {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		want[m] = true
	}
	useAll := want["all"]

	type imp struct{ alias, path string }
	impSeen := map[string]bool{}
	var imports []imp
	type reg struct{ module, exportExpr string }
	var regs []reg

	// Global callables are always registered wholesale for the full embedded
	// runtime (len/cap override the builtin table's versions with the yak-aware
	// runtimeYakBuiltinLen/Cap). Registration order matters: tables first, then
	// the len/cap override (later assignments win in runtimeRegisterYaklibGlobals).
	globalImp := func(alias, path string) {
		if !impSeen[alias] {
			impSeen[alias] = true
			imports = append(imports, imp{alias, path})
		}
	}
	globalImp("yaklib", "github.com/yaklang/yaklang/common/yak/yaklib")
	globalImp("builtin", "github.com/yaklang/yaklang/common/yak/yaklang/lib/builtin")
	type globalReg struct{ expr string }
	var globalRegs []globalReg
	globalRegs = append(globalRegs, globalReg{"yaklib.GlobalExport"}, globalReg{"builtin.YaklangBaseLib"})
	globalRegs = append(globalRegs, globalReg{`map[string]any{"len": runtimeYakBuiltinLen, "cap": runtimeYakBuiltinCap}`})

	for _, name := range AllModuleNames() {
		if !useAll && !want[name] {
			continue
		}
		spec, ok := LookupModuleSpec(name)
		if !ok {
			continue
		}
		sources := spec.prunedExportSources()
		if len(sources) == 0 {
			continue
		}
		for _, src := range sources {
			if !impSeen[src.ImportAlias] {
				impSeen[src.ImportAlias] = true
				imports = append(imports, imp{src.ImportAlias, src.GoImportPath})
			}
			regs = append(regs, reg{name, src.ExportExpr})
		}
	}

	// Group export exprs per module (composite modules have several sources).
	byMod := map[string][]string{}
	var modNames []string
	for _, r := range regs {
		if _, ok := byMod[r.module]; !ok {
			modNames = append(modNames, r.module)
		}
		byMod[r.module] = append(byMod[r.module], r.exportExpr)
	}
	sort.Strings(modNames)

	sortedImports := make([]imp, len(imports))
	copy(sortedImports, imports)
	sort.Slice(sortedImports, func(i, j int) bool { return sortedImports[i].alias < sortedImports[j].alias })

	var b strings.Builder
	b.WriteString("package main\n\n")
	if len(sortedImports) > 0 {
		b.WriteString("import (\n")
		for _, im := range sortedImports {
			b.WriteString(fmt.Sprintf("\t%s %q\n", im.alias, im.path))
		}
		b.WriteString(")\n\n")
	}
	b.WriteString("func init() {\n")
	for _, g := range globalRegs {
		b.WriteString(fmt.Sprintf("\truntimeRegisterYaklibGlobals(%s)\n", g.expr))
	}
	for _, m := range modNames {
		for _, expr := range byMod[m] {
			b.WriteString(fmt.Sprintf("\truntimeRegisterYaklibModule(%q, %s)\n", m, expr))
		}
	}
	b.WriteString("}\n")

	return os.WriteFile(outputPath, []byte(b.String()), 0o644)
}
