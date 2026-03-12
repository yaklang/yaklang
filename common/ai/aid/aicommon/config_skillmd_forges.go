package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func (c *Config) loadSkillMDForgesIntoSkillLoader() {
	if c == nil || c.disableAutoSkills {
		return
	}
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return
	}
	forges, err := yakit.GetAIForgesByType(db, schema.FORGE_TYPE_SkillMD)
	if err != nil {
		log.Warnf("failed to query skillmd forges from DB: %v", err)
		return
	}
	if len(forges) == 0 {
		return
	}
	fSys, count, err := aiskillloader.BuildSkillSourceFSFromForges(forges)
	if err != nil {
		log.Warnf("failed to build skill filesystem from skillmd forges: %v", err)
		return
	}
	if count == 0 {
		return
	}
	loader := c.ensureSkillLoader()
	if loader == nil {
		return
	}
	if added, err := loader.AddSource(fSys); err != nil {
		log.Warnf("failed to add skillmd forge source into skill loader: %v", err)
	} else if added > 0 {
		log.Debugf("auto-loaded %d skills from %d skillmd forges", added, count)
	}
}
