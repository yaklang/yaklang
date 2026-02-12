package aireact

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

//go:embed skills
var builtinSkillsFS embed.FS

// GetBuiltinSkillsFS returns the embedded filesystem containing built-in skills.
// These skills ship with the binary and are always available unless explicitly
// disabled via WithDisableAutoSkills(true).
//
// The filesystem root contains skill directories (e.g. skills/code-review/),
// each with a SKILL.md defining the skill metadata and content.
func GetBuiltinSkillsFS() fi.FileSystem {
	return filesys.NewEmbedFS(builtinSkillsFS)
}
