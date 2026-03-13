//go:build !ssa2llvm_gzip_embed

package runtimeembed

func embeddedRuntimeFS() (readFileFS, bool) {
	return nil, false
}

func embeddedRuntimeSourceFS() (readDirFileFS, bool) {
	return nil, false
}
