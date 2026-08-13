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
// prunedExportSources are skipped; unknown names are ignored. When aot is true
// the generated file prefers each module's PrunedShim (lightweight AOT export
// tables) so monolithic yaklib-backed modules do not pull the full yaklib
// package into the AOT runtime.
func WriteRuntimeImportsPerModule(outputPath string, modules []string, aot bool) error {
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

	// Global registration is provided by runtime_globals_aot.go /
	// runtime_globals_full.go (build tag ssa2llvm_aot): the AOT runtime must
	// not import the monolithic common/yak/yaklib or yaklang builtin packages,
	// otherwise the whole yaklang frontend stack (typescript/java/php/python,
	// goja, ssaapi, ...) is pulled into every binary. Module export tables are
	// registered per module below; the full (non-AOT) build keeps the original
	// yaklib.GlobalExport + builtin.YaklangBaseLib registration.
	globalImps := []imp{
		{"_", "unsafe"},
	}
	for _, gi := range globalImps {
		if !impSeen[gi.alias] {
			impSeen[gi.alias] = true
			imports = append(imports, gi)
		}
	}

	for _, name := range AllModuleNames() {
		if !useAll && !want[name] {
			continue
		}
		spec, ok := LookupModuleSpec(name)
		if !ok || len(spec.prunedExportSources()) == 0 {
			continue
		}
		for _, src := range aotExportSources(spec, aot) {
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
	// Empty init: modules are imported for their Go init tasks, but registration
	// is deferred to the exported functions below.
	b.WriteString("func init() {}\n\n")
	// Global builtins registration.
	b.WriteString("//export yak_register_globals\n")
	b.WriteString("func yak_register_globals() {\n")
	b.WriteString("\tregisterRuntimeGlobals()\n")
	b.WriteString("}\n\n")
	writePrunedModuleStubs(&b, modNames)
	// Per-module registration only reads the package export tables. The Go
	// runtime executes init tasks for used modules through their retained
	// inittask relocations; patching marks unused tasks done before linking.
	// Calling a Go init function manually through linkname would execute the
	// init ABI a second time and can corrupt runtime state.
	for _, m := range modNames {
		exprs := byMod[m]
		b.WriteString(fmt.Sprintf("//export yak_register_module_%s\n", m))
		b.WriteString(fmt.Sprintf("func yak_register_module_%s() {\n", m))
		for _, expr := range exprs {
			b.WriteString(fmt.Sprintf("\truntimeRegisterYaklibModule(%q, %s)\n", m, expr))
		}
		b.WriteString("}\n\n")
	}
	return os.WriteFile(outputPath, []byte(b.String()), 0o644)
}

// SplitOnlyGroups are elfsplit section groups that carry code but are not
// yaklang modules: "shared" is the dependency closure every module needs and
// "ssafront" holds the language frontends behind the ssa module. They get the
// same pruned-module stubs as real modules so a wrong dependency closure fails
// with their name too.
var SplitOnlyGroups = []string{"shared", "sharednet", "ssafront"}

// writePrunedModuleStubs emits one stub per module group. Compile-time pruning
// redirects every relocation that referenced an unused .modtext.<module>
// function to that module's stub, which keeps the stub reachable (so lld does
// not collect it) and gives stale function pointers a defined target.
//
// The stub panics rather than returning. A no-op stub makes a wrong dependency
// closure fail silently — the call returns, the caller reads uninitialized
// result registers, and the damage surfaces arbitrarily far away. Panicking
// with the module name turns that into a report naming the module to add.
func writePrunedModuleStubs(b *strings.Builder, modNames []string) {
	seen := map[string]bool{}
	var groups []string
	for _, m := range append(append([]string{}, modNames...), SplitOnlyGroups...) {
		if m != "" && !seen[m] {
			seen[m] = true
			groups = append(groups, m)
		}
	}
	sort.Strings(groups)

	b.WriteString("//go:noinline\n")
	b.WriteString("func yakPrunedModulePanic(module string) {\n")
	b.WriteString("\tpanic(\"yaklib module was pruned at link time but is still reachable: \" + module +\n")
	b.WriteString("\t\t\" (the compiler's used-module closure is missing it)\")\n")
	b.WriteString("}\n\n")
	// Retained for archives patched by an older toolchain, which looks this
	// symbol up by name and has no per-module fallback.
	b.WriteString("//export yakUnusedModuleStub\n")
	b.WriteString("//go:noinline\n")
	b.WriteString("func yakUnusedModuleStub() { yakPrunedModulePanic(\"<unknown>\") }\n\n")
	for _, m := range groups {
		b.WriteString(fmt.Sprintf("//export yakPrunedModuleStub_%s\n", m))
		b.WriteString("//go:noinline\n")
		b.WriteString(fmt.Sprintf("func yakPrunedModuleStub_%s() { yakPrunedModulePanic(%q) }\n\n", m, m))
	}
}

func aotExportSources(spec ModuleImportSpec, aot bool) []ExportSource {
	if spec.PrunedShim != nil {
		if aot || len(spec.regularExportSources()) == 0 {
			return []ExportSource{*spec.PrunedShim}
		}
		return spec.regularExportSources()
	}
	return spec.regularExportSources()
}
