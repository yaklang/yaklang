//go:build selfcontained

package compiler

import (
	"fmt"
	"path/filepath"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/embed/assets"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

// This file provides the self-contained compile + link backend used when ssa2llvm
// is built with -tags=selfcontained. It replaces the llc/clang subprocess calls
// (compiler/linker.go, build tag !selfcontained) with in-process go-llvm
// TargetMachine emission (replacing llc) and in-process lld linking of the
// embedded runtime archives + crt + static libc (replacing clang). The result is
// a portable, fully-static AOT executable produced with zero external toolchain
// dependency at runtime.

// newNativeTargetMachine creates a TargetMachine for the host triple with a
// generic CPU (no host-specific features) so emitted code runs on any cpu of
// the target arch. RelocStatic + CodeModelSmall match a non-PIE static
// executable linked by lld with -nostdlib -static.
func newNativeTargetMachine() (llvm.TargetMachine, error) {
	if err := llvm.InitializeNativeTarget(); err != nil {
		return llvm.TargetMachine{}, fmt.Errorf("init native target: %w", err)
	}
	if err := llvm.InitializeNativeAsmPrinter(); err != nil {
		return llvm.TargetMachine{}, fmt.Errorf("init native asm printer: %w", err)
	}
	triple := llvm.DefaultTargetTriple()
	tm, err := llvm.NewTargetMachine(triple, "", "", llvm.CodeGenLevelDefault, llvm.RelocStatic, llvm.CodeModelSmall)
	if err != nil {
		return llvm.TargetMachine{}, fmt.Errorf("create target machine for %q: %w", triple, err)
	}
	return tm, nil
}

// CompileModuleToAsmSC emits assembly from an in-memory Module via go-llvm
// TargetMachine (replaces `llc`). No subprocess is spawned; the trace line is a
// synthetic equivalent command for -x visibility.
func CompileModuleToAsmSC(mod llvm.Module, asmFile string) error {
	tm, err := newNativeTargetMachine()
	if err != nil {
		return err
	}
	defer tm.Dispose()
	mod.ApplyTargetMachine(tm)
	trace.PrintLink("llc (in-process, TargetMachine)", []string{
		"-filetype=asm", "-mtriple=" + tm.Triple(), "-o", asmFile, "<in-memory module>",
	})
	return tm.EmitToFile(mod, asmFile, llvm.AssemblyFile)
}

// CompileModuleToObjectSC emits a relocatable object from an in-memory Module
// via go-llvm TargetMachine (replaces `llc -filetype=obj`). No subprocess is
// spawned; the trace line is a synthetic equivalent command for -x visibility.
func CompileModuleToObjectSC(mod llvm.Module, objFile string) error {
	tm, err := newNativeTargetMachine()
	if err != nil {
		return err
	}
	defer tm.Dispose()
	mod.ApplyTargetMachine(tm)
	trace.PrintLink("llc (in-process, TargetMachine)", []string{
		"-filetype=obj", "-mtriple=" + tm.Triple(), "-o", objFile, "<in-memory module>",
	})
	return tm.EmitToFile(mod, objFile, llvm.ObjectFile)
}

// CompileObjectToBinarySC links an object + the embedded runtime archives
// (libyak.a, libgc.a) + crt objects (crt1/crti/crtbegin ... crtend/crtn) +
// static system libs (libc.a, libgcc.a, libgcc_eh.a) into a portable, fully
// static executable via in-process lld (replaces `clang` linking). workDir is
// where embedded assets are released (the compile work dir, cleaned by the
// caller). obfArchives are additional on-disk archives (e.g. obfuscation
// runtime deps); they are not yet embedded in v1.
func CompileObjectToBinarySC(objFile, binFile, workDir string, obfArchives []string, extraArgs ...string) error {
	rp, err := assets.ReleaseTo(workDir)
	if err != nil {
		return err
	}
	archives := make([]string, 0, 2+len(obfArchives))
	archives = append(archives, rp.Libyak, rp.Libgc)
	archives = append(archives, obfArchives...)

	in := llvm.StaticLinkInput{
		ObjectPath: objFile,
		Archives:   archives,
		CRTBegin:   []string{rp.Crt1, rp.Crti, rp.CrtBegin},
		CRTEnd:     []string{rp.CrtEnd, rp.Crtn},
		SystemLibs: []string{rp.Libc, rp.Libgcc, rp.LibgccEh},
		OutputPath: binFile,
		ExtraArgs:  extraArgs,
	}
	trace.PrintLink("ld.lld (in-process)", llvm.StaticLinkArgs(in))
	if err := llvm.LinkExecutableStatic(in); err != nil {
		return fmt.Errorf("self-contained link failed: %w", err)
	}
	return nil
}

// resolveSCWorkDir returns the directory to release embedded assets into. It
// prefers cfg.WorkDir; callers pass that explicitly via CompileObjectToBinarySC.
func resolveSCWorkDir(workDir, binFile string) string {
	if workDir != "" {
		return workDir
	}
	return filepath.Dir(binFile)
}