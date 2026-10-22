package aicommon

import "github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"

// GetSkillsContextManager 返回 session 级技能操作入口，不依赖当前是否有活跃 loop。
// 创建/绑定只接入当前 Timeline，不加载默认技能或补写历史正文。
// loop 仍创建自己的 manager；窗口信息在使用时从共同的 Timeline 按并集补齐。
func (c *Config) GetSkillsContextManager() *aiskillloader.SkillsContextManager {
	if c == nil {
		return nil
	}
	c.skillsContextManagerMu.Lock()
	defer c.skillsContextManagerMu.Unlock()
	loader := c.GetSkillLoader()
	if c.skillsContextManager == nil || c.skillsContextManager.GetLoader() != loader {
		c.skillsContextManager = aiskillloader.NewSkillsContextManager(loader)
	}
	c.skillsContextManager.BindTimelineSink(NewSkillContentTimelineAdapter(c.GetTimeline(), c.AcquireId))
	return c.skillsContextManager
}
