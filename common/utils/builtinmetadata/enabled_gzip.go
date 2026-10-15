//go:build gzip_embed

package builtinmetadata

import "github.com/yaklang/yaklang/common/consts"

const Enabled = true

func embeddedMetadata(kind string) ([]byte, string, error) { return consts.BuiltinMetadata(kind) }
