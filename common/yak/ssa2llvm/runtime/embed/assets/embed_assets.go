package assets

import "embed"

// Embedded assets populated by scripts/build_runtime_embed.sh. The files must
// exist in this directory at build time (run the script before building ssa2llvm).

//go:embed libyak.a
var embeddedLibyak []byte

//go:embed libgc.a
var embeddedLibgc []byte

//go:embed crt1.o
var embeddedCrt1 []byte

//go:embed crti.o
var embeddedCrti []byte

//go:embed crtn.o
var embeddedCrtn []byte

//go:embed crtbegin.o
var embeddedCrtBegin []byte

//go:embed crtend.o
var embeddedCrtEnd []byte

//go:embed libc.a
var embeddedLibc []byte

//go:embed libgcc.a
var embeddedLibgcc []byte

//go:embed libgcc_eh.a
var embeddedLibgccEh []byte

// embeddedExtDeps holds the extra cgo C static libraries (libpcap.a, libm.a,
// libresolv.a, ...) that the registered yaklib modules pull in. The set is
// variable (depends on SSA2LLVM_EMBED_MODULES), so the whole extdeps/ directory
// is embedded; extdepManifest (generated alongside EmbeddedManifest) lists which
// members to release/link and their SHA256. Populated by build_runtime_embed.sh.
//
//go:embed extdeps
var embeddedExtDeps embed.FS

// EmbeddedAvailable is true when the real embedded assets are compiled in.
const EmbeddedAvailable = true
