package aicommon

import (
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"sync/atomic"
)

func (c *Config) GetTimeline() *Timeline {
	return c.Timeline
}

func (c *Config) GetAIForgeManager() AIForgeFactory {
	return c.AiForgeManager
}

// LookupAIForgeForInvoke returns the forge definition used when invoking a blueprint by name.
// Resolution order matches aireact.ReAct.getForgeByName: ExtendedForge first, then AiForgeManager.
func (c *Config) LookupAIForgeForInvoke(forgeName string) (*schema.AIForge, error) {
	if c == nil {
		return nil, utils.Error("config is nil")
	}
	if forgeName == "" {
		return nil, utils.Error("forge name is empty")
	}
	for _, forge := range c.ExtendedForge {
		if forge != nil && forge.ForgeName == forgeName {
			return forge, nil
		}
	}
	if c.AiForgeManager == nil {
		return nil, utils.Error("AiForgeManager is not configured")
	}
	return c.AiForgeManager.GetAIForge(forgeName)
}

func (c *Config) GetForgeName() string {
	return c.ForgeName
}

func (c *Config) GetInputConsumption() int64 {
	state := c.ensureConsumptionState()
	if state == nil {
		return 0
	}
	input, _ := state.GetConsumptionPointers()
	if input == nil {
		return 0
	}
	return atomic.LoadInt64(input)
}

func (c *Config) GetOutputConsumption() int64 {
	state := c.ensureConsumptionState()
	if state == nil {
		return 0
	}
	_, output := state.GetConsumptionPointers()
	if output == nil {
		return 0
	}
	return atomic.LoadInt64(output)
}

func (c *Config) GetCacheHitToken() int64 {
	state := c.ensureConsumptionState()
	if state == nil {
		return 0
	}
	cacheHit := state.GetCacheHitTokenPointer()
	if cacheHit == nil {
		return 0
	}
	return atomic.LoadInt64(cacheHit)
}

func (c *Config) GetSequenceStart() int64 {
	return c.Seq
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

func (c *Config) GetDisableIntentRecognition() bool {
	return c.DisableIntentRecognition
}

func (c *Config) GetSyncPerceptionTrigger() bool {
	return c.SyncPerceptionTrigger
}

func (c *Config) GetAiToolManager() *buildinaitools.AiToolManager {
	return c.AiToolManager
}

func (c *Config) GetTopToolsCount() int {
	return c.TopToolsCount
}

func (c *Config) GetShowForgeListInPrompt() bool {
	return c.ShowForgeListInPrompt
}

func (c *Config) GetMaxIterations() int64 {
	return c.MaxIterationCount
}

func (c *Config) GetEnableSelfReflection() bool {
	return c.EnableSelfReflection
}

func (c *Config) GetToolCallIntervalReviewExtraPrompt() string {
	return c.ToolCallIntervalReviewExtraPrompt
}
