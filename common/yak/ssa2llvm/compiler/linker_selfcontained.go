package compiler

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/embed/assets"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/patch"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

// This file provides the compile + link backend: in-process go-llvm
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

// CompileObjectToBinarySCWithPatch links an object + the embedded runtime
// archives, optionally patching the released libyak.a so lld --gc-sections can
// drop yaklib modules not used by the script (usedModules lists the modules the
// script uses; all others are candidates for removal).
func CompileObjectToBinarySCWithPatch(objFile, binFile, workDir string, obfArchives []string, usedModules []string, extraArgs ...string) error {
	return linkStaticWithPatch(objFile, binFile, workDir, obfArchives, usedModules, "", extraArgs...)
}

// CompileObjectToBinarySCTier links like CompileObjectToBinarySCWithPatch but
// against the archive of the named tier (see runtime/tiers) instead of the
// embedded one. Pruning removes module code from whichever archive is used;
// the tier decides how much module *metadata* the archive has in the first
// place. An empty tier, or one with no installed archive, uses the embedded
// archive, which is larger but always correct.
func CompileObjectToBinarySCTier(objFile, binFile, workDir string, obfArchives []string, usedModules []string, tier string, extraArgs ...string) error {
	return linkStaticWithPatch(objFile, binFile, workDir, obfArchives, usedModules, tier, extraArgs...)
}

// CompileObjectToBinarySC links an object + the embedded runtime archives
// (libyak.a, libgc.a) + crt objects (crt1/crti/crtbegin ... crtend/crtn) +
// static system libs (libc.a, libgcc.a, libgcc_eh.a) into a portable, fully
// static executable via in-process lld (replaces `clang` linking). workDir is
// where embedded assets are released (the compile work dir, cleaned by the
// caller). obfArchives are additional on-disk archives (e.g. obfuscation
// runtime deps); they are not yet embedded in v1.
func CompileObjectToBinarySC(objFile, binFile, workDir string, obfArchives []string, extraArgs ...string) error {
	return linkStaticWithPatch(objFile, binFile, workDir, obfArchives, nil, "", extraArgs...)
}

func linkStaticWithPatch(objFile, binFile, workDir string, obfArchives []string, usedModules []string, tier string, extraArgs ...string) error {
	used := append([]string{}, usedModules...)
	for attempt := 0; attempt < 8; attempt++ {
		rp, resolved, err := assets.ReleaseTierTo(workDir, tier)
		if err != nil {
			return err
		}
		if attempt == 0 {
			traceTierChoice(resolved)
		}
		// A module the archive cannot register is an undefined symbol at link
		// time, and the in-process lld dies on SIGSEGV while reporting it
		// instead of returning an error. Check the object against the
		// archive's symbol index first, so this ends in a sentence naming the
		// module. The inputs do not change between attempts.
		if attempt == 0 {
			if err := checkModulesAvailable(objFile, rp.Libyak); err != nil {
				return err
			}
		}
		// Compile-time yaklib pruning: remove references into unused .modtext
		// sections so lld --gc-sections can drop those modules from the final
		// binary. Always invoked; Patch itself no-ops when there are no split
		// sections or every module is used.
		if err := patch.Patch(patch.Request{ArchivePath: rp.Libyak, UsedModules: used}); err != nil {
			return fmt.Errorf("patch libyak: %w", err)
		}
		archives := make([]string, 0, 2+len(obfArchives)+len(rp.ExtDeps))
		archives = append(archives, rp.Libyak, rp.Libgc)
		archives = append(archives, obfArchives...)
		// Extra cgo C static libraries the registered yaklib modules pull in
		// (e.g. libpcap.a for poc, libm.a, libresolv.a). They are embedded (see
		// assets) so the link stays zero-host-dependency. They go inside the
		// --start-group with the runtime + system libs so mutual references
		// resolve regardless of order.
		archives = append(archives, rp.ExtDeps...)

		linkArgs := append([]string{}, extraArgs...)
		// Link-time size reduction (no libyak.a change): drop unreferenced
		// sections and strip debug info + the symbol table. Safe for a static
		// AOT executable (Go reflection uses its own rodata itab/typelinks, not
		// the ELF symtab).
		//
		// --icf=safe is deliberately NOT used. ld.lld's safe ICF needs the
		// .llvm_addrsig section clang emits with -faddrsig; neither go.o nor the
		// GCC-built system archives have it, so lld treats every symbol as
		// address-significant and folds nothing. Worse, if it did fold, two
		// identical Go functions would share one address and runtime.textsectmap
		// would attribute the folded PC to the wrong function.
		linkArgs = append(linkArgs, "--gc-sections")
		in := llvm.StaticLinkInput{
			ObjectPath: objFile,
			Archives:   archives,
			CRTBegin:   []string{rp.Crt1, rp.Crti, rp.CrtBegin},
			CRTEnd:     []string{rp.CrtEnd, rp.Crtn},
			SystemLibs: []string{rp.Libc, rp.Libgcc, rp.LibgccEh},
			OutputPath: binFile,
			ExtraArgs:  linkArgs,
		}
		trace.PrintLink("ld.lld (in-process)", llvm.StaticLinkArgs(in))
		if err := llvm.LinkExecutableStatic(in); err != nil {
			return fmt.Errorf("self-contained link failed: %w", err)
		}
		// elfsplit emits runtime.textsectmap entries in original vaddr order,
		// but runtime.pcToOffset requires them sorted by final physical base
		// address. Sort the table in the linked binary so traceback/findfunc
		// work with the packed .text and .modtext.* layout.
		if err := patch.SortFinalTextMap(binFile); err != nil {
			return fmt.Errorf("sort final textsectmap: %w", err)
		}
		// The base runtime's init graph can reference a module the script does
		// not use. patch then clears that module's textmap relocations, but lld
		// keeps the section, breaking PC lookup. Re-link with such modules
		// marked used until the retained set matches the textmap coverage.
		missing, err := patch.MissingRetainedModules(binFile)
		if err != nil {
			return fmt.Errorf("check retained module textmap: %w", err)
		}
		added := false
		for _, m := range missing {
			if !slices.Contains(used, m) {
				used = append(used, m)
				added = true
			}
		}
		if len(missing) == 0 || !added {
			return nil
		}
	}
	return fmt.Errorf("self-contained link did not converge on retained module set")
}

// traceTierChoice reports which runtime archive the link used. A fallback is
// worth seeing under -x: the build is correct but the binary is bigger than the
// script needs, and installing the wanted tier is what fixes it.
func traceTierChoice(r assets.ResolvedTier) {
	switch {
	case r.Wanted == "":
		return
	case r.Fallback():
		trace.Printf("runtime tier: wanted %q, using %q (%s); put a %s/libyak.a under $%s for a smaller binary",
			r.Wanted, r.Used, r.Source, r.Wanted, assets.TierDirEnv)
	default:
		trace.Printf("runtime tier: %q (%s)", r.Used, r.Source)
	}
}

// checkModulesAvailable fails when the embedded libyak.a was built without a
// module the script uses. The embedded archive carries a fixed module set
// (SSA2LLVM_EMBED_MODULES at build time), so a script reaching for anything
// outside it can only be fixed by rebuilding the runtime.
func checkModulesAvailable(objPath, archivePath string) error {
	missing, err := patch.MissingModuleRegistrations(objPath, archivePath)
	if err != nil || len(missing) == 0 {
		return err
	}
	available, err := patch.ArchiveModules(archivePath)
	if err != nil {
		return err
	}
	return fmt.Errorf("the embedded runtime has no yaklib module %q; it was built with: %s\n"+
		"rebuild it including the missing module(s):\n"+
		"  SSA2LLVM_EMBED_MODULES=%s bash common/yak/ssa2llvm/scripts/build_yaklib.sh",
		strings.Join(missing, ", "), strings.Join(available, ", "),
		strings.Join(append(available, missing...), ","))
}

// resolveSCWorkDir returns the directory to release embedded assets into. It
// prefers cfg.WorkDir; callers pass that explicitly via CompileObjectToBinarySC.
func resolveSCWorkDir(workDir, binFile string) string {
	if workDir != "" {
		return workDir
	}
	return filepath.Dir(binFile)
}
