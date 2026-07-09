package embed

import "strings"

// WriteRuntimeImports generates a runtime_imports_generated.go that imports
// and registers the given yaklib modules, so a libyak.a built from it has those
// modules' exports available at runtime.
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

	deps := make([]YaklibDependency, 0)
	for _, name := range AllModuleNames() {
		if !useAll && !want[name] {
			continue
		}
		spec, ok := LookupModuleSpec(name)
		if !ok || len(spec.prunedExportSources()) == 0 {
			continue
		}
		deps = append(deps, YaklibDependency{Module: name})
	}
	return writePrunedRuntimeImportsToFile(outputPath, deps)
}