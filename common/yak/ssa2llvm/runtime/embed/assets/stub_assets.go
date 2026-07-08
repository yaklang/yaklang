//go:build !selfcontained

package assets

// Stub placeholders used when the build is not self-contained (no
// -tags=selfcontained). ReleaseTo returns an error in this mode; the legacy
// clang/llc link path is used instead.

var (
	embeddedLibyak   []byte
	embeddedLibgc    []byte
	embeddedCrt1     []byte
	embeddedCrti     []byte
	embeddedCrtn     []byte
	embeddedCrtBegin []byte
	embeddedCrtEnd   []byte
	embeddedLibc     []byte
	embeddedLibgcc   []byte
	embeddedLibgccEh []byte
)

const EmbeddedAvailable = false

var EmbeddedManifest = Manifest{}