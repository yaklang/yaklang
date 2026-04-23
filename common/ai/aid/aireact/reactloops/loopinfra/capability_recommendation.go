package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func skillLoaderFromLoop(loop *reactloops.ReActLoop) aiskillloader.SkillLoader {
	if loop == nil {
		return nil
	}
	if mgr := loop.GetSkillsContextManager(); mgr != nil {
		return mgr.GetLoader()
	}
	return nil
}

func buildForgePromptCorpus(forge *schema.AIForge) string {
	if forge == nil {
		return ""
	}

	var sections []string
	appendSection := func(title, content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		sections = append(sections, fmt.Sprintf("[%s]\n%s", title, content))
	}

	appendSection("InitPrompt", forge.InitPrompt)
	appendSection("PersistentPrompt", forge.PersistentPrompt)
	appendSection("PlanPrompt", forge.PlanPrompt)
	appendSection("ResultPrompt", forge.ResultPrompt)

	return strings.Join(sections, "\n\n")
}

func getForgeForRecommendation(invoker aicommon.AIInvokeRuntime, forgeName string) *schema.AIForge {
	if invoker == nil || strings.TrimSpace(forgeName) == "" {
		return nil
	}

	type forgeManagerProvider interface {
		GetAIForgeManager() aicommon.AIForgeFactory
	}
	cfg := invoker.GetConfig()
	if provider, ok := cfg.(forgeManagerProvider); ok {
		if forgeMgr := provider.GetAIForgeManager(); forgeMgr != nil {
			forge, err := forgeMgr.GetAIForge(forgeName)
			if err == nil && forge != nil {
				return forge
			}
		}
	}

	forge, err := yakit.GetAIForgeByName(consts.GetGormProfileDatabase(), forgeName)
	if err != nil {
		return nil
	}
	return forge
}

func recommendCapabilitiesFromMatches(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, matches *reactloops.CapabilityNameMatchResult, sourceLabel string) string {
	if loop == nil || invoker == nil || matches == nil || !matches.HasMatches() {
		return ""
	}

	reactloops.PopulateExtraCapabilitiesFromCapabilityMatches(invoker, loop, matches)
	summary := matches.Summary()
	if summary != "" {
		invoker.AddToTimeline("capability_match_recommendation",
			fmt.Sprintf("%s references capabilities: %s", sourceLabel, summary))
		log.Infof("%s references capabilities: %s", sourceLabel, summary)
	}
	return summary
}

func recommendCapabilitiesFromForgePrompts(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, forgeName string, sourceLabel string) string {
	forge := getForgeForRecommendation(invoker, forgeName)
	if forge == nil {
		return ""
	}
	corpus := buildForgePromptCorpus(forge)
	if corpus == "" {
		return ""
	}
	if loop != nil {
		loop.LoadingStatus("正在匹配相关能力 / Matching related capabilities...")
	}
	matches := reactloops.MatchCapabilitiesByText(corpus, skillLoaderFromLoop(loop))
	return recommendCapabilitiesFromMatches(loop, invoker, matches, sourceLabel)
}

func recommendCapabilitiesFromSkillContent(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, skillName string, sourceLabel string) string {
	loader := skillLoaderFromLoop(loop)
	if loader == nil {
		return ""
	}
	loaded, err := loader.LoadSkill(skillName)
	if err != nil || loaded == nil {
		return ""
	}
	content := strings.TrimSpace(loaded.SkillMDContent)
	if content == "" {
		return ""
	}
	if loop != nil {
		loop.LoadingStatus("正在匹配相关能力 / Matching related capabilities...")
	}
	matches := reactloops.MatchCapabilitiesByText(content, loader)
	return recommendCapabilitiesFromMatches(loop, invoker, matches, sourceLabel)
}
