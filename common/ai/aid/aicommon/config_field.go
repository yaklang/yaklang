package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"sync/atomic"
)

func (c *Config) GetTimeline() *Timeline  {
	return c.Timeline
}

func (c *Config) GetAIForgeManager() AIForgeFactory {
	return c.AiForgeManager
}

func (c *Config) GetInputConsumption() int64 {
	return atomic.LoadInt64(c.InputConsumption)
}

func (c *Config) GetOutputConsumption() int64 {
	return atomic.LoadInt64(c.OutputConsumption)
}

func (c *Config) InputConsumptionCallback(current int) {
	atomic.AddInt64(c.InputConsumption, int64(current))
}

func (c *Config) GetSequenceStart() int64 {
	return c.IdSequence
}

func (c *Config) GetLanguage() string {
	return c.Language
}

func (c *Config) GetEnablePlanAndExec() bool {
	return c.EnablePlanAndExec
}

func (c *Config) GetEnableUserInteract() bool {
	return c.AllowRequireForUserInteract
}

func (c *Config) GetEnhanceKnowledgeManager() *EnhanceKnowledgeManager {
	return c.EnhanceKnowledgeManager
}

func (c *Config) GetDisableEnhanceDirectlyAnswer() bool {
	return c.DisableEnhanceDirectlyAnswer
}

func (c *Config) GetAiToolManager() *buildinaitools.AiToolManager {
	return c.AiToolManager
}

func (c *Config) GetTopToolsCount() int {
	return c.TopToolsCount
}

func (c *Config) GetMaxIterations() int64 {
	return c.MaxIterationCount
}

func (c *Config) GetEnableSelfReflection() bool  {
	return c.EnableSelfReflection
}