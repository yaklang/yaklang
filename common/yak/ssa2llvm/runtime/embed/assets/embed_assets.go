//go:build selfcontained

package assets

import _ "embed"

// Embedded assets populated by scripts/build_runtime_embed.sh. The files must
// exist in this directory at build time (run the script before
// `go build -tags=selfcontained`).

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

// EmbeddedAvailable is true when the real embedded assets are compiled in.
const EmbeddedAvailable = true