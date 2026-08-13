package compiler

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/tiers"
)

// emitAsmModule emits assembly in-process via go-llvm TargetMachine from the
// in-memory Module (replaces the external `llc` tool).
func emitAsmModule(mod llvm.Module, finalLL, asmFile string) error {
	return CompileModuleToAsmSC(mod, asmFile)
}

// emitObjectModule emits an object file in-process via go-llvm TargetMachine
// from the in-memory Module (replaces the external `llc -filetype=obj`).
func emitObjectModule(mod llvm.Module, finalLL, objFile string) error {
	return CompileModuleToObjectSC(mod, objFile)
}

// resolveDiskRuntimeArchive returns "" under the self-contained build: the
// runtime is embedded, never read from disk.
func resolveDiskRuntimeArchive(cfg *CompileConfig) (string, error) {
	return "", nil
}

// prepareAndLinkBinary compiles the in-memory Module to an object (TargetMachine)
// and links it with the embedded runtime archives + crt + static system libs
// into a portable, fully-static executable in-process via lld. The embedded
// runtime is always used; pruned-runtime generation, disk archives, and linkprep
// symbol randomization are not applicable in this mode. Returns empty
// runtimeArchive/extraLinkArgs (the runtime is embedded, so there is nothing
// host-specific to record for the cache beyond the embedded manifest).
func prepareAndLinkBinary(comp *Compiler, finalLL, outputFile string, cfg *CompileConfig) (string, []string, error) {
	// Runtime symbol randomization (profile link_prep) needs external ar/objcopy/nm
	// (see linkprep/archive.go), which is incompatible with the zero-dep self-contained
	// mode and is therefore not supported.
	if len(cfg.RuntimeSymManifest) > 0 {
		return "", nil, fmt.Errorf("runtime symbol randomization (link_prep) is not supported in the zero-dependency self-contained mode")
	}
	objPath := filepath.Join(cfg.WorkDir, "ssa2llvm-out.o")
	if err := CompileModuleToObjectSC(comp.Mod, objPath); err != nil {
		return "", nil, err
	}
	// Determine yaklib modules the script actually uses; pass them to the
	// linker so unused modules are pruned from libyak.a before lld runs.
	usedModules := make([]string, 0)
	deps := comp.YaklibDependencies()
	for mod := range deps {
		usedModules = append(usedModules, mod)
	}
	// The tier is chosen from the script's own modules, before the closure
	// below adds the groups those modules drag in. Groups are sections inside
	// an archive; tiers are whole archives, and only yaklib module names name
	// a rung of the ladder.
	tier := selectRuntimeTier(usedModules)
	// Module dependency closure for per-module DCE. "shared" (schema/lowhttp/
	// net-http/gorm closure) is required whenever any yaklib module is used;
	// the poc package uses cli at runtime, and the ssa module needs its
	// language frontends. Without these, patch redirects the cross-module
	// references to the stub and the kept module crashes or loses functions.
	if len(usedModules) > 0 {
		usedModules = append(usedModules, "shared")
	}
	for _, dep := range moduleGroupDeps {
		if !containsModule(usedModules, dep.module) {
			continue
		}
		for _, need := range dep.needs {
			if !containsModule(usedModules, need) {
				usedModules = append(usedModules, need)
			}
		}
	}
	if err := CompileObjectToBinarySCTier(objPath, outputFile, cfg.WorkDir, cfg.ObfArchives, usedModules, tier, cfg.ExtraLinkArgs...); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}

// selectRuntimeTier picks the smallest pre-built archive that can register the
// script's modules. A module outside the ladder is not an error here: the
// embedded archive may still carry it (SSA2LLVM_EMBED_MODULES takes any list),
// and if it does not, checkModulesAvailable says so by name before lld runs.
func selectRuntimeTier(scriptModules []string) string {
	if len(scriptModules) == 0 {
		return tiers.All[0].Name
	}
	t, err := tiers.Select(scriptModules)
	if err != nil {
		return ""
	}
	return t.Name
}

// moduleGroupDeps is what a used module drags in besides its own section.
//
// One module's code can call another's, and both call into the split shared
// closure. `go list -deps` on the modules' entry packages is what says so:
// common/utils/cli (the cli module) is in the closure of http, poc and ssa,
// and common/utils/lowhttp/poc is in the closure of http and ssa. Without an
// entry here the call lands in the missing group's stub — at run time, in
// whatever code path first needs it.
//
// "sharednet" holds what the codec/os/str shims never touch (mostly the
// network, TLS and database stacks), so a pure-computation script does not pay
// for it. Its module list must stay equal to the one elfsplit generated into
// generatedSharedGroupModules["sharednet"].
var moduleGroupDeps = []struct {
	module string
	needs  []string
}{
	{"poc", []string{"cli", "sharednet"}},
	{"http", []string{"cli", "poc", "sharednet"}},
	{"cli", []string{"sharednet"}},
	{"ssa", []string{"cli", "poc", "ssafront", "sharednet"}},
}

// moduleClosureKey identifies the rules above for the build cache. A cached
// binary was linked with one particular closure; changing the rules has to
// invalidate it, or the next build silently reuses a binary missing a module.
func moduleClosureKey() string {
	var b strings.Builder
	for _, dep := range moduleGroupDeps {
		b.WriteString(dep.module)
		b.WriteString("=>")
		b.WriteString(strings.Join(dep.needs, "+"))
		b.WriteString(";")
	}
	return b.String()
}

func containsModule(modules []string, want string) bool {
	for _, m := range modules {
		if m == want {
			return true
		}
	}
	return false
}
