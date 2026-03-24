package aicommon

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

func (c *Config) loadSkillMDForgesIntoSkillLoader() {
	if c == nil || c.disableAutoSkills {
		return
	}
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return
	}
	loader := c.ensureSkillLoader()
	if loader == nil {
		return
	}
	if added, err := loader.AddDatabase(db); err != nil {
		log.Warnf("failed to attach skillmd forge source into skill loader: %v", err)
	} else if added > 0 {
		log.Debugf("attached skillmd forge source with %d available skills", added)
	}
}
