package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"runtime/debug"
	"strconv"
	"sync"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/embed/assets"
)

// cacheToolKeyPart writes the self-contained build's key components: a hash of
// the embedded runtime manifest (libyak/libgc/crt/libc/libgcc/libgcc_eh SHA256s)
// plus the bundled LLVM version. No host toolchain is probed and no subprocess
// is spawned, so the cache key is stable across machines with the same embedded
// runtime and the build has zero external dependencies at compile time.
func cacheToolKeyPart(cfg *CompileConfig, write func(string)) {
	m := assets.EmbeddedManifest
	h := sha256.New()
	add := func(s string) {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	add("libyak=" + m.Libyak)
	add("libgc=" + m.Libgc)
	add("crt1=" + m.Crt1)
	add("crti=" + m.Crti)
	add("crtn=" + m.Crtn)
	add("crtbegin=" + m.CrtBegin)
	add("crtend=" + m.CrtEnd)
	add("libc=" + m.Libc)
	add("libgcc=" + m.Libgcc)
	add("libgcc_eh=" + m.LibgccEh)
	// Include the variable set of extra cgo C static libraries (libpcap.a, ...)
	// so the cache key reflects which module deps are embedded. extdepManifest is
	// generated in stable order by build_runtime_embed.sh.
	for _, ed := range assets.ExtdepManifest() {
		add("extdep:" + ed.Name + "=" + ed.Sha)
	}
	write("embeddedRuntime=" + hex.EncodeToString(h.Sum(nil)))
	write("llvm=" + bundledLLVMVersion())
	// Reflect the link-time options applied by CompileObjectToBinarySC so a
	// change in stripping/gc-sections invalidates cached artifacts.
	write("linkOpts=gc-sections+strip")
	// Which modules survive the link is decided by these rules, so a change
	// to them produces a different binary from the same script.
	write("moduleClosure=" + moduleClosureKey())
	// Which runtime archive the link lands on depends on the tier archives
	// installed on this machine, not just on the embedded one.
	write("tiers=" + assets.TierStateKey())
	// A forced tier and keep-all mode change what the same script produces,
	// so they are part of the artifact identity.
	write("tierOverride=" + cfg.TierOverride)
	write("keepAllModules=" + strconv.FormatBool(cfg.KeepAllModules))
}

// bundledLLVMVersion identifies the in-process LLVM/lld that produced the
// cached artifact. The go-llvm module version is the only identifier that
// actually tracks the bundled toolchain: a hardcoded string silently keeps
// stale artifacts alive across an LLVM upgrade.
var bundledLLVMVersion = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown-selfcontained"
	}
	for _, dep := range info.Deps {
		if dep.Path == goLLVMModulePath {
			return dep.Version + "-selfcontained"
		}
	}
	return "unknown-selfcontained"
})

const goLLVMModulePath = "github.com/yaklang/go-llvm"
