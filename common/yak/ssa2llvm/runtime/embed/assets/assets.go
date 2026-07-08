// Package assets holds the embedded runtime archives (libyak.a, libgc.a), crt
// objects, and static system libraries (libc.a, libgcc.a, libgcc_eh.a) that make
// the ssa2llvm binary self-contained at runtime: when ssa2llvm compiles a yak
// script it links these in-process via lld (see github.com/yaklang/go-llvm)
// instead of shelling out to clang/llc or reading artifacts from the host.
//
// The real embedded bytes are present only when this package is built with the
// `selfcontained` build tag (embed_assets.go). Without the tag, stub_assets.go
// provides empty placeholders and ReleaseTo returns an error, so the default
// dev/test build keeps using the legacy clang/llc path. The distributed
// self-contained binary is built with -tags=selfcontained after running
// scripts/build_runtime_embed.sh to populate the asset files.
package assets

import (
	"fmt"
	"os"
	"path/filepath"
)

// Manifest records the SHA256 of each embedded asset. It is populated by
// manifest_generated.go (build tag `selfcontained`) from the build script and
// used as a compile-cache key so cache validity is independent of the host
// toolchain. Without the tag it is the zero value.
type Manifest struct {
	Libyak     string
	Libgc      string
	Crt1       string
	Crti       string
	Crtn       string
	CrtBegin   string
	CrtEnd     string
	Libc       string
	Libgcc     string
	LibgccEh   string
}

// ReleasedPaths holds the absolute paths of assets written to the work dir by
// ReleaseTo.
type ReleasedPaths struct {
	Libyak   string
	Libgc    string
	Crt1     string
	Crti     string
	Crtn     string
	CrtBegin string
	CrtEnd   string
	Libc     string
	Libgcc   string
	LibgccEh string
}

// EmbeddedAvailable and EmbeddedManifest are defined per build tag in
// embed_assets.go / stub_assets.go / manifest_generated.go.
//
//	const EmbeddedAvailable bool
//	var   EmbeddedManifest Manifest
//
// embeddedLibyak / embeddedLibgc / embeddedCrt1 / embeddedCrti / embeddedCrtn /
// embeddedCrtBegin / embeddedCrtEnd / embeddedLibc / embeddedLibgcc /
// embeddedLibgccEh are the asset bytes (real under the tag, empty stub otherwise).

// HasEmbeddedRuntime reports whether the embedded runtime archives are
// available in this build (i.e. built with -tags=selfcontained).
func HasEmbeddedRuntime() bool { return EmbeddedAvailable }

func writeAsset(dir, name string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("embedded asset %q is empty; rebuild with -tags=selfcontained after running scripts/build_runtime_embed.sh", name)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return "", fmt.Errorf("release embedded asset %q: %w", name, err)
	}
	return p, nil
}

// ReleaseTo writes all embedded runtime assets into dir and returns their
// absolute paths. dir should be the compile work dir (the caller owns its
// cleanup). Returns an error if this build was not compiled with the
// `selfcontained` tag.
func ReleaseTo(dir string) (ReleasedPaths, error) {
	if !EmbeddedAvailable {
		return ReleasedPaths{}, fmt.Errorf("embedded runtime not available: rebuild ssa2llvm with -tags=selfcontained after running scripts/build_runtime_embed.sh")
	}
	var rp ReleasedPaths
	var err error
	if rp.Libyak, err = writeAsset(dir, "libyak.a", embeddedLibyak); err != nil {
		return rp, err
	}
	if rp.Libgc, err = writeAsset(dir, "libgc.a", embeddedLibgc); err != nil {
		return rp, err
	}
	if rp.Crt1, err = writeAsset(dir, "crt1.o", embeddedCrt1); err != nil {
		return rp, err
	}
	if rp.Crti, err = writeAsset(dir, "crti.o", embeddedCrti); err != nil {
		return rp, err
	}
	if rp.Crtn, err = writeAsset(dir, "crtn.o", embeddedCrtn); err != nil {
		return rp, err
	}
	if rp.CrtBegin, err = writeAsset(dir, "crtbegin.o", embeddedCrtBegin); err != nil {
		return rp, err
	}
	if rp.CrtEnd, err = writeAsset(dir, "crtend.o", embeddedCrtEnd); err != nil {
		return rp, err
	}
	if rp.Libc, err = writeAsset(dir, "libc.a", embeddedLibc); err != nil {
		return rp, err
	}
	if rp.Libgcc, err = writeAsset(dir, "libgcc.a", embeddedLibgcc); err != nil {
		return rp, err
	}
	if rp.LibgccEh, err = writeAsset(dir, "libgcc_eh.a", embeddedLibgccEh); err != nil {
		return rp, err
	}
	return rp, nil
}