//go:build !ssa2llvm_gzip_embed

package embed

func EmbeddedRuntimeHash() (string, bool, error) {
	return "", false, nil
}

func EmbeddedRuntimeSourceHash() (string, bool, error) {
	return "", false, nil
}
