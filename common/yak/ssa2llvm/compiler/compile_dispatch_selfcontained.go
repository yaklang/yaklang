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
	if cfg.TierOverride != "" {
		if t, ok := tiers.Lookup(cfg.TierOverride); ok {
			tier = t.Name
		} else {
			return "", nil, fmt.Errorf("unknown runtime tier %q (available: %s)", cfg.TierOverride, tiers.Names())
		}
	}
	if cfg.KeepAllModules {
		t, ok := tiers.Lookup(tier)
		if !ok {
			return "", nil, fmt.Errorf("keep-all-modules needs a known runtime tier, got %q", tier)
		}
		usedModules = append(usedModules[:0], t.Modules...)
	}
	// Module dependency closure for per-module DCE. "shared" (schema/lowhttp/
	// net-http/gorm closure) is required whenever any yaklib module is used;
	// the poc package uses cli at runtime, and the ssa module needs its
	// language frontends. Without these, patch redirects the cross-module
	// references to the stub and the kept module crashes or loses functions.
	if len(usedModules) > 0 {
		usedModules = append(usedModules, "shared")
	}
	// Close over the dependency table until it stops growing: entries can
	// depend on each other (ssa <-> ai), so a single pass would miss groups
	// added by an entry that was already visited.
	for changed := true; changed; {
		changed = false
		for _, dep := range moduleGroupDeps {
			if !containsModule(usedModules, dep.module) {
				continue
			}
			for _, need := range dep.needs {
				if !containsModule(usedModules, need) {
					usedModules = append(usedModules, need)
					changed = true
				}
			}
		}
	}
	// The staticanalyze tier's modules share the monolithic yaklib and the
	// network/SSA closures so pervasively that pruning the shared groups
	// inside it is not sound: yaklib-backed modules call sharednet at runtime,
	// sca/omnisearch reach ssafront, and the ai/liteforge/sandbox modules
	// cross-register init callbacks. Keep the whole shared closure for this
	// tier; the core and net tiers still prune per script.
	if tier == "staticanalyze" {
		for _, g := range []string{"sharednet", "ssafront", "ssa", "cli", "poc", "ai", "tools", "httptpl"} {
			if !containsModule(usedModules, g) {
				usedModules = append(usedModules, g)
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
	// Global callables (die, sprintf, ...) are recorded with an empty module;
	// they are runtime globals, not tier modules, so drop them before lookup.
	filtered := make([]string, 0, len(scriptModules))
	for _, mod := range scriptModules {
		if mod != "" {
			filtered = append(filtered, mod)
		}
	}
	if len(filtered) == 0 {
		return tiers.All[0].Name
	}
	t, err := tiers.Select(filtered)
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
	{"ssa", []string{"cli", "poc", "ssafront", "sharednet", "ai"}},
	// The ai/liteforge/sandbox/rag/dyn/hook modules share the common/yak
	// monolith and the common/ai subtree, whose package init functions
	// register callbacks across those packages (aiconfig -> yakit, aiforge ->
	// aitool, common/yak -> aicommon). The groups must therefore stay
	// together: ai drags ssa (yakit), and the yak-monolith-backed modules
	// drag ai (common/yak lives in the ai group).
	{"ai", []string{"ssa"}},
	{"liteforge", []string{"ai"}},
	{"sandbox", []string{"ai"}},
	{"rag", []string{"ai"}},
	{"dyn", []string{"ai"}},
	{"hook", []string{"ai"}},
	// Modules whose export tables live in the ssa group (their own packages
	// are placed there by elfsplit) must keep it, or the module registration
	// function calls a pruned stub at start-up.
	{"simulator", []string{"ssa"}},
	{"suricata", []string{"ssa"}},
	{"pprof", []string{"ssa"}},
	// The nuclei module is backed by the same httptpl package as the httptpl
	// module, so using it must keep the httptpl group.
	{"nuclei", []string{"httptpl"}},
	// Monolithic-yaklib-backed modules: their export tables live in
	// common/yak/yaklib, which elfsplit places in the ssafront group.
	{"atoi", []string{"ssafront"}},
	{"bot", []string{"ssafront"}},
	{"bufio", []string{"ssafront"}},
	{"context", []string{"ssafront"}},
	{"csrf", []string{"ssafront"}},
	{"db", []string{"ssafront"}},
	{"dictutil", []string{"ssafront"}},
	{"dns", []string{"ssafront"}},
	{"dnslog", []string{"ssafront"}},
	{"env", []string{"ssafront"}},
	{"exec", []string{"ssafront"}},
	{"filemonitor", []string{"ssafront"}},
	{"filescanner", []string{"ssafront"}},
	{"fuzz", []string{"ssafront"}},
	{"fuzzx", []string{"ssafront"}},
	{"gzip", []string{"ssafront"}},
	{"httpool", []string{"ssafront"}},
	{"httpserver", []string{"ssafront"}},
	{"io", []string{"ssafront"}},
	{"js", []string{"ssafront"}},
	{"jsonstream", []string{"ssafront"}},
	{"ldap", []string{"ssafront"}},
	{"math", []string{"ssafront"}},
	{"mitm", []string{"ssafront"}},
	{"mmdb", []string{"ssafront"}},
	{"rdp", []string{"ssafront"}},
	{"re", []string{"ssafront"}},
	{"re2", []string{"ssafront"}},
	{"redis", []string{"ssafront"}},
	{"regen", []string{"ssafront"}},
	{"risk", []string{"ssafront"}},
	{"smb", []string{"ssafront"}},
	{"spacengine", []string{"ssafront"}},
	{"ssh", []string{"ssafront"}},
	{"tcp", []string{"ssafront"}},
	{"timezone", []string{"ssafront"}},
	{"tls", []string{"ssafront"}},
	{"traceroute", []string{"ssafront"}},
	{"udp", []string{"ssafront"}},
	{"x", []string{"ssafront"}},
	{"xml", []string{"ssafront"}},
	{"yaml", []string{"ssafront"}},
	{"zip", []string{"ssafront"}},
	// Tools-backed modules: their export tables live in
	// common/yak/yaklib/tools, which elfsplit places in the tools group.
	{"brute", []string{"tools"}},
	{"finscan", []string{"tools"}},
	{"ping", []string{"tools"}},
	{"servicescan", []string{"tools"}},
	{"subdomain", []string{"tools"}},
	{"synscan", []string{"tools"}},
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
