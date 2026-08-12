package compiler

import (
	"fmt"
	"path/filepath"

	"github.com/yaklang/go-llvm"
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
	if err := CompileObjectToBinarySCWithPatch(objPath, outputFile, cfg.WorkDir, cfg.ObfArchives, usedModules, cfg.ExtraLinkArgs...); err != nil {
		return "", nil, err
	}
	return "", nil, nil
}
