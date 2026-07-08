//go:build !selfcontained

package compiler

import (
	"path/filepath"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/linkprep"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/embed"
)

// emitAsmModule emits assembly via the external `llc` tool from a .ll file.
// (selfcontained build uses go-llvm TargetMachine instead.)
func emitAsmModule(mod llvm.Module, finalLL, asmFile string) error {
	return CompileLLVMToAsm(finalLL, asmFile)
}

// emitObjectModule emits an object file via the external `llc` tool from a .ll
// file. (selfcontained build uses go-llvm TargetMachine instead.)
func emitObjectModule(mod llvm.Module, finalLL, objFile string) error {
	return CompileLLVMToObject(finalLL, objFile)
}

// resolveDiskRuntimeArchive returns a prebuilt runtime archive from disk when
// stdlib pruning is disabled. The self-contained build embeds the runtime
// instead and returns "".
func resolveDiskRuntimeArchive(cfg *CompileConfig) (string, error) {
	return findRuntimeArchive()
}

// prepareAndLinkBinary builds/resolves the runtime archive (pruned build or
// disk), runs linkprep symbol randomization when requested, and links the final
// executable via the external `clang` tool. Returns the runtime archive path and
// extra link args for the compile result/cache.
func prepareAndLinkBinary(comp *Compiler, finalLL, outputFile string, cfg *CompileConfig) (string, []string, error) {
	runtimeArchive := strings.TrimSpace(cfg.RuntimeArchive)
	extraLinkArgs := append([]string{}, cfg.ExtraLinkArgs...)
	linkingNative := !cfg.SkipRuntimeLink

	if linkingNative && strings.TrimSpace(runtimeArchive) == "" && cfg.StdlibCompile {
		deps := runtimeDepsFromCompiler(comp)
		archivePath, gcLibDir, buildErr := embed.BuildPrunedRuntimeArchiveFromLocalSourceWithDeps(cfg.WorkDir, deps)
		if buildErr != nil {
			return "", nil, buildErr
		}
		runtimeArchive = archivePath
		cfg.RuntimeArchive = archivePath
		if strings.TrimSpace(gcLibDir) != "" {
			extraLinkArgs = append(extraLinkArgs, "-L"+gcLibDir)
			cfg.ExtraLinkArgs = extraLinkArgs
		}
	}
	if linkingNative && len(cfg.RuntimeSymManifest) > 0 && strings.TrimSpace(cfg.RuntimeArchive) != "" {
		archives := []string{filepath.Clean(cfg.RuntimeArchive)}
		for _, o := range cfg.ObfArchives {
			o = strings.TrimSpace(o)
			if o != "" {
				archives = append(archives, filepath.Clean(o))
			}
		}
		out, cleanup, lpErr := linkprep.PrepareForLink(linkprep.PrepareInput{
			Archives: archives,
			Manifest: cfg.RuntimeSymManifest,
			WorkDir:  cfg.WorkDir,
			Trace:    cfg.Trace,
		})
		if lpErr != nil {
			return "", nil, utils.Errorf("linkprep: %v", lpErr)
		}
		defer cleanup()
		cfg.RuntimeArchive = out[0]
		if len(out) > 1 {
			cfg.ObfArchives = append([]string{}, out[1:]...)
		}
	}

	if err := CompileLLVMToBinary(finalLL, outputFile, !cfg.SkipRuntimeLink, cfg.RuntimeArchive, cfg.ObfArchives, cfg.ExtraLinkArgs...); err != nil {
		return "", nil, err
	}
	return runtimeArchive, extraLinkArgs, nil
}