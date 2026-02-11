package aiskillloader

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// NewLocalSkillLoader creates a SkillLoader from a local directory path.
// The directory should contain skill subdirectories at its root level.
//
// Example:
//
//	/path/to/skills/
//	  deploy-app/
//	    SKILL.md
//	  code-review/
//	    SKILL.md
func NewLocalSkillLoader(dirPath string) (*FSSkillLoader, error) {
	if !utils.IsDir(dirPath) {
		return nil, utils.Errorf("skill directory does not exist: %s", dirPath)
	}
	localFS := filesys.NewRelLocalFs(dirPath)
	return NewFSSkillLoader(localFS)
}
