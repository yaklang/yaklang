//go:build selfcontained

package compiler

import (
	"crypto/sha256"
	"encoding/hex"

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
	write("embeddedRuntime=" + hex.EncodeToString(h.Sum(nil)))
	write("llvm=18.1.3-selfcontained")
}