//go:build !ssa2llvm_gzip_embed

package embed

func embeddedRuntimeFS() (readFileFS, bool) {
	return nil, false
}

func embeddedRuntimeSourceFS() (readDirFileFS, bool) {
	return nil, false
}
