//go:build !gzip_embed

package aireact

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed skills/***
var builtinSkillsEmbedFS embed.FS

func InitBuiltinSkillsFS() {
	builtinSkillsFS = resources_monitor.NewStandardResourceMonitor(builtinSkillsEmbedFS, "")
}

func init() {
	InitBuiltinSkillsFS()
}
