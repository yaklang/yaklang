//go:build !gzip_embed

package runtimeembed

func embeddedRuntimeFS() (readFileFS, bool) {
	return nil, false
}
