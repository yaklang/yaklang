//go:build !gzip_embed

package builtinmetadata

import "fmt"

const Enabled = false

func embeddedMetadata(kind string) ([]byte, string, error) {
	return nil, "", fmt.Errorf("metadata is disabled")
}
