//go:build gzip_embed

package builtinmetadata

import "github.com/yaklang/yaklang/common/consts"

const Enabled = true

var embeddedCatalog = []byte(consts.BuiltinMetadataCatalog)
