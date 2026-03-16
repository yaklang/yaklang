//go:build ssa2llvm_gzip_embed

package embed

// EmbeddedRuntimeHash returns the content hash of the embedded runtime archive
// (ssa2llvm-runtime.tar.gz).
func EmbeddedRuntimeHash() (string, bool, error) {
	if runtimeFS == nil {
		return "", false, nil
	}
	h, err := runtimeFS.GetHash()
	return h, true, err
}

// EmbeddedRuntimeSourceHash returns the content hash of the embedded runtime
// source archive (ssa2llvm-runtime-src.tar.gz).
func EmbeddedRuntimeSourceHash() (string, bool, error) {
	if runtimeSourceFS == nil {
		return "", false, nil
	}
	h, err := runtimeSourceFS.GetHash()
	return h, true, err
}
