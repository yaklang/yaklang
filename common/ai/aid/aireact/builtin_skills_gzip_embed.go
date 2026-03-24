//go:build gzip_embed

package aireact

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/resources_monitor"
)

//go:embed skills.tar.gz
var builtinSkillsEmbedFS embed.FS

func InitBuiltinSkillsFS() {
	builtinSkillsFS = resources_monitor.NewGzipResourceMonitor(&builtinSkillsEmbedFS, "skills.tar.gz", "skills")
}

func init() {
	InitBuiltinSkillsFS()
}
