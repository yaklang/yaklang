package reactloops

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type reActLoopCapabilityContext ReActLoop

func (r *ReActLoop) capabilityInventoryContext() aicommon.CapabilityInventoryLoopContext {
	if r == nil {
		return nil
	}
	return (*reActLoopCapabilityContext)(r)
}

func (r *reActLoopCapabilityContext) PromptCandidateTools() []*aitool.Tool {
	return (*ReActLoop)(r).GetPromptCandidateTools()
}

func (r *reActLoopCapabilityContext) ScenarioToolWhitelist() []string {
	return (*ReActLoop)(r).GetScenarioToolWhitelist()
}

func (r *reActLoopCapabilityContext) AllowToolCall() bool {
	loop := (*ReActLoop)(r)
	if loop.allowToolCall == nil {
		return true
	}
	return loop.allowToolCall()
}

func (r *reActLoopCapabilityContext) DynamicExtraTools() []*aitool.Tool {
	extra := (*ReActLoop)(r).GetExtraCapabilities()
	if extra == nil {
		return nil
	}
	return extra.ListTools()
}

func (r *reActLoopCapabilityContext) DynamicForges() []aicommon.CapabilityInventoryNamedItem {
	extra := (*ReActLoop)(r).GetExtraCapabilities()
	if extra == nil {
		return nil
	}
	result := make([]aicommon.CapabilityInventoryNamedItem, 0, len(extra.ListForges()))
	for _, forge := range extra.ListForges() {
		if strings.TrimSpace(forge.Name) == "" {
			continue
		}
		result = append(result, aicommon.CapabilityInventoryNamedItem{
			Name:        forge.Name,
			VerboseName: forge.VerboseName,
			Description: forge.Description,
			Category:    "forge",
		})
	}
	return result
}

func (r *reActLoopCapabilityContext) LoadedSkills() []aicommon.CapabilityInventoryNamedItem {
	mgr := (*ReActLoop)(r).GetSkillsContextManager()
	if mgr == nil {
		return nil
	}
	return buildLoadedSkillItems(mgr.GetCurrentSelectedSkills())
}

func BuildCapabilityInventoryPayload(cfg *aicommon.Config, loop *ReActLoop) aicommon.CapabilityInventoryPayload {
	return aicommon.BuildCapabilityInventoryPayload(cfg, loop.capabilityInventoryContext())
}

func buildLoadedSkillItems(selected []*aiskillloader.SkillMeta) []aicommon.CapabilityInventoryNamedItem {
	result := make([]aicommon.CapabilityInventoryNamedItem, 0, len(selected))
	for _, item := range selected {
		if item == nil || strings.TrimSpace(item.Name) == "" {
			continue
		}
		result = append(result, aicommon.CapabilityInventoryNamedItem{
			Name:        item.Name,
			VerboseName: item.Name,
			Description: item.Description,
			Category:    "skill",
		})
	}
	return result
}

func EmitCapabilityInventorySnapshot(cfg *aicommon.Config, loop *ReActLoop) {
	if cfg == nil || cfg.GetEmitter() == nil {
		return
	}
	_, _ = cfg.GetEmitter().EmitSystemStructured(
		aicommon.CapabilityInventoryNodeID,
		BuildCapabilityInventoryPayload(cfg, loop),
	)
}
