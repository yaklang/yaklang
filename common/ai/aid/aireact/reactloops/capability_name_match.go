package reactloops

import (
	"context"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type CapabilityNameMatchResult struct {
	MatchedYakScripts []*schema.YakScript
	MatchedAITools    []*schema.AIYakTool
	MatchedForges     []*schema.AIForge
	MatchedSkills     []*aiskillloader.SkillMeta
}

type capabilityNameMatchCandidate struct {
	YakScript *schema.YakScript
	AITool    *schema.AIYakTool
	Forge     *schema.AIForge
	Skill     *aiskillloader.SkillMeta
}

func MatchCapabilitiesByText(input string, skillLoader aiskillloader.SkillLoader) *CapabilityNameMatchResult {
	normalizedInput := strings.ToLower(strings.TrimSpace(input))
	if normalizedInput == "" {
		return nil
	}

	patterns, candidatesByPattern := collectCapabilityNameMatchCandidates(skillLoader)
	if len(patterns) == 0 {
		return nil
	}

	result := matchCapabilityCandidatesByIndexAllSubstrings(normalizedInput, patterns, candidatesByPattern)
	if !result.HasMatches() {
		return nil
	}
	return result
}

func collectCapabilityNameMatchCandidates(skillLoader aiskillloader.SkillLoader) ([]string, map[string][]capabilityNameMatchCandidate) {
	var patterns []string
	patternSeen := make(map[string]bool)
	candidatesByPattern := make(map[string][]capabilityNameMatchCandidate)

	addCandidate := func(candidate string, matchCandidate capabilityNameMatchCandidate) {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate == "" {
			return
		}
		if !patternSeen[candidate] {
			patternSeen[candidate] = true
			patterns = append(patterns, candidate)
		}
		candidatesByPattern[candidate] = append(candidatesByPattern[candidate], matchCandidate)
	}

	if db := consts.GetGormProfileDatabase(); db != nil {
		collectYakScriptNameMatchCandidates(db, addCandidate)
		collectAIToolNameMatchCandidates(db, addCandidate)
		collectForgeNameMatchCandidates(db, addCandidate)
	}
	collectSkillNameMatchCandidates(skillLoader, addCandidate)

	return patterns, candidatesByPattern
}

func MatchCapabilitiesByTextWithConfig(config aicommon.AICallerConfigIf, input string) *CapabilityNameMatchResult {
	EmitCapabilityMatchingStatus(config)
	return MatchCapabilitiesByText(input, resolveSkillLoaderFromConfig(config))
}

func EmitCapabilityMatchingStatus(config aicommon.AICallerConfigIf) {
	if config == nil || config.GetEmitter() == nil {
		return
	}
	config.GetEmitter().EmitStatus(ReActLoadingStatusKey, "正在匹配相关能力 / Matching related capabilities...")
}

func resolveSkillLoaderFromConfig(config aicommon.AICallerConfigIf) aiskillloader.SkillLoader {
	if config == nil {
		return nil
	}
	type skillLoaderProvider interface {
		GetSkillLoader() aiskillloader.SkillLoader
	}
	if provider, ok := config.(skillLoaderProvider); ok {
		return provider.GetSkillLoader()
	}
	return nil
}

func collectYakScriptNameMatchCandidates(db *gorm.DB, addCandidate func(string, capabilityNameMatchCandidate)) {
	scriptDB := db.Model(&schema.YakScript{}).
		Select("script_name, type, ai_desc, help, ai_keywords, enable_for_ai").
		Where("script_name <> ''").
		Where("enable_for_ai = ?", true)

	for script := range yakit.YieldYakScripts(scriptDB, context.Background()) {
		if script == nil {
			continue
		}
		scriptName := strings.TrimSpace(script.ScriptName)
		if scriptName == "" {
			continue
		}
		addCandidate(scriptName, capabilityNameMatchCandidate{YakScript: script})
	}
}

func collectAIToolNameMatchCandidates(db *gorm.DB, addCandidate func(string, capabilityNameMatchCandidate)) {
	for tool := range yakit.YieldAllAITools(context.Background(), db) {
		if tool == nil {
			continue
		}
		toolName := strings.TrimSpace(tool.Name)
		if toolName == "" {
			continue
		}
		candidate := capabilityNameMatchCandidate{AITool: tool}
		addCandidate(toolName, candidate)
		addCandidate(tool.VerboseName, candidate)
		addCandidate(tool.Path, candidate)
	}
}

func collectForgeNameMatchCandidates(db *gorm.DB, addCandidate func(string, capabilityNameMatchCandidate)) {
	for forge := range yakit.YieldAllAIForges(context.Background(), db) {
		if forge == nil {
			continue
		}
		forgeName := strings.TrimSpace(forge.ForgeName)
		if forgeName == "" {
			continue
		}
		candidate := capabilityNameMatchCandidate{Forge: forge}
		addCandidate(forgeName, candidate)
		addCandidate(forge.ForgeVerboseName, candidate)
	}
}

func collectSkillNameMatchCandidates(skillLoader aiskillloader.SkillLoader, addCandidate func(string, capabilityNameMatchCandidate)) {
	if skillLoader == nil || !skillLoader.HasSkills() {
		return
	}

	for _, meta := range skillLoader.AllSkillMetas() {
		if meta == nil {
			continue
		}
		skillName := strings.TrimSpace(meta.Name)
		if skillName == "" {
			continue
		}
		addCandidate(skillName, capabilityNameMatchCandidate{Skill: meta})
	}
}

func matchCapabilityCandidatesByIndexAllSubstrings(
	normalizedInput string,
	patterns []string,
	candidatesByPattern map[string][]capabilityNameMatchCandidate,
) *CapabilityNameMatchResult {
	result := &CapabilityNameMatchResult{}

	seenYakScripts := make(map[string]bool)
	seenAITools := make(map[string]bool)
	seenForges := make(map[string]bool)
	seenSkills := make(map[string]bool)

	matches := utils.IndexAllSubstrings(normalizedInput, patterns...)
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		patternIndex := match[0]
		if patternIndex < 0 || patternIndex >= len(patterns) {
			continue
		}
		pattern := patterns[patternIndex]
		for _, candidate := range candidatesByPattern[pattern] {
			if script := candidate.YakScript; script != nil {
				name := strings.TrimSpace(script.ScriptName)
				if name != "" && !seenYakScripts[name] {
					seenYakScripts[name] = true
					result.MatchedYakScripts = append(result.MatchedYakScripts, script)
				}
			}
			if tool := candidate.AITool; tool != nil {
				name := strings.TrimSpace(tool.Name)
				if name != "" && !seenAITools[name] {
					seenAITools[name] = true
					result.MatchedAITools = append(result.MatchedAITools, tool)
				}
			}
			if forge := candidate.Forge; forge != nil {
				name := strings.TrimSpace(forge.ForgeName)
				if name != "" && !seenForges[name] {
					seenForges[name] = true
					result.MatchedForges = append(result.MatchedForges, forge)
				}
			}
			if skill := candidate.Skill; skill != nil {
				name := strings.TrimSpace(skill.Name)
				if name != "" && !seenSkills[name] {
					seenSkills[name] = true
					result.MatchedSkills = append(result.MatchedSkills, skill)
				}
			}
		}
	}

	return result
}

func (r *CapabilityNameMatchResult) HasMatches() bool {
	return r != nil && (len(r.MatchedYakScripts) > 0 || len(r.MatchedAITools) > 0 || len(r.MatchedForges) > 0 || len(r.MatchedSkills) > 0)
}

func (r *CapabilityNameMatchResult) ToolNames() []string {
	if r == nil {
		return nil
	}
	var names []string
	for _, script := range r.MatchedYakScripts {
		if script != nil && script.EnableForAI {
			names = append(names, script.ScriptName)
		}
	}
	for _, tool := range r.MatchedAITools {
		if tool != nil {
			names = append(names, tool.Name)
		}
	}
	return normalizeCapabilityStrings(names)
}

func (r *CapabilityNameMatchResult) ForgeNames() []string {
	if r == nil {
		return nil
	}
	var names []string
	for _, forge := range r.MatchedForges {
		if forge != nil {
			names = append(names, forge.ForgeName)
		}
	}
	return normalizeCapabilityStrings(names)
}

func (r *CapabilityNameMatchResult) SkillNames() []string {
	if r == nil {
		return nil
	}
	var names []string
	for _, skill := range r.MatchedSkills {
		if skill != nil {
			names = append(names, skill.Name)
		}
	}
	return normalizeCapabilityStrings(names)
}

func (r *CapabilityNameMatchResult) Summary() string {
	if !r.HasMatches() {
		return ""
	}

	var parts []string
	if tools := r.ToolNames(); len(tools) > 0 {
		parts = append(parts, "tools["+strings.Join(tools, ",")+"]")
	}
	if forges := r.ForgeNames(); len(forges) > 0 {
		parts = append(parts, "blueprints["+strings.Join(forges, ",")+"]")
	}
	if skills := r.SkillNames(); len(skills) > 0 {
		parts = append(parts, "skills["+strings.Join(skills, ",")+"]")
	}
	var pluginNames []string
	for _, script := range r.MatchedYakScripts {
		if script != nil && !script.EnableForAI {
			pluginNames = append(pluginNames, script.ScriptName)
		}
	}
	if pluginNames = normalizeCapabilityStrings(pluginNames); len(pluginNames) > 0 {
		parts = append(parts, "plugins["+strings.Join(pluginNames, ",")+"]")
	}

	return strings.Join(parts, " ")
}

func (r *CapabilityNameMatchResult) RenderMarkdown(title string) string {
	if !r.HasMatches() {
		return ""
	}

	var builder strings.Builder
	if title != "" {
		builder.WriteString(title)
		builder.WriteString("\n")
	}

	if len(r.MatchedYakScripts) > 0 {
		builder.WriteString("#### Matched Yakit Plugins\n")
		for _, script := range r.MatchedYakScripts {
			if script == nil {
				continue
			}
			desc := script.AIDesc
			if desc == "" {
				desc = script.Help
			}
			desc = utils.ShrinkString(desc, 200)
			builder.WriteString(fmt.Sprintf("- **[%s] %s**", strings.ToUpper(script.Type), script.ScriptName))
			if desc != "" {
				builder.WriteString(": ")
				builder.WriteString(desc)
			}
			builder.WriteString(" [mentioned in content]")
			if script.AIKeywords != "" {
				builder.WriteString(fmt.Sprintf(" [keywords: %s]", script.AIKeywords))
			}
			if !script.EnableForAI {
				builder.WriteString(" [name match only; not AI-enabled]")
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}

	if len(r.MatchedAITools) > 0 {
		builder.WriteString("#### Matched AI Tools\n")
		for _, tool := range r.MatchedAITools {
			if tool == nil {
				continue
			}
			name := tool.Name
			if tool.VerboseName != "" {
				name = tool.VerboseName + " (" + tool.Name + ")"
			}
			builder.WriteString(fmt.Sprintf("- **%s**: %s [mentioned in content]\n", name, utils.ShrinkString(tool.Description, 200)))
		}
		builder.WriteString("\n")
	}

	if len(r.MatchedForges) > 0 {
		builder.WriteString("#### Matched Blueprints\n")
		for _, forge := range r.MatchedForges {
			if forge == nil {
				continue
			}
			name := forge.ForgeName
			if forge.ForgeVerboseName != "" {
				name = forge.ForgeVerboseName + " (" + forge.ForgeName + ")"
			}
			builder.WriteString(fmt.Sprintf("- **%s**: %s [mentioned in content]\n", name, utils.ShrinkString(forge.Description, 200)))
		}
		builder.WriteString("\n")
	}

	if len(r.MatchedSkills) > 0 {
		builder.WriteString("#### Matched Skills\n")
		for _, skill := range r.MatchedSkills {
			if skill == nil {
				continue
			}
			builder.WriteString(fmt.Sprintf("- **%s**: %s [mentioned in content]\n", skill.Name, utils.ShrinkString(skill.Description, 200)))
		}
		builder.WriteString("\n")
	}

	return strings.TrimSpace(builder.String())
}

func (r *CapabilityNameMatchResult) RenderYakScriptMarkdown(title string) string {
	if r == nil || len(r.MatchedYakScripts) == 0 {
		return ""
	}

	var builder strings.Builder
	if title != "" {
		builder.WriteString(title)
		builder.WriteString("\n")
	}
	for _, script := range r.MatchedYakScripts {
		if script == nil {
			continue
		}
		desc := script.AIDesc
		if desc == "" {
			desc = script.Help
		}
		desc = utils.ShrinkString(desc, 200)
		builder.WriteString(fmt.Sprintf("- **[%s] %s**", strings.ToUpper(script.Type), script.ScriptName))
		if desc != "" {
			builder.WriteString(": ")
			builder.WriteString(desc)
		}
		builder.WriteString(" [mentioned in content]")
		if script.AIKeywords != "" {
			builder.WriteString(fmt.Sprintf(" [keywords: %s]", script.AIKeywords))
		}
		if !script.EnableForAI {
			builder.WriteString(" [name match only; not AI-enabled]")
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func mergeCapabilityNames(base string, extras ...string) string {
	merged := append(normalizeCapabilityNames(base), extras...)
	merged = normalizeCapabilityStrings(merged)
	return strings.Join(merged, ",")
}

func ApplyCapabilityMatchesToDeepIntentResult(result *DeepIntentResult, matches *CapabilityNameMatchResult) {
	if result == nil || matches == nil || !matches.HasMatches() {
		return
	}

	result.MatchedCapabilityMentions = matches

	if toolNames := matches.ToolNames(); len(toolNames) > 0 {
		result.MatchedToolNames = mergeCapabilityNames(result.MatchedToolNames, toolNames...)
		toolLine := "Matched named capabilities: " + strings.Join(toolNames, ",")
		if strings.TrimSpace(result.RecommendedTools) != "" {
			result.RecommendedTools = strings.TrimSpace(result.RecommendedTools) + "\n" + toolLine
		} else {
			result.RecommendedTools = toolLine
		}
	}

	if forgeNames := matches.ForgeNames(); len(forgeNames) > 0 {
		result.MatchedForgeNames = mergeCapabilityNames(result.MatchedForgeNames, forgeNames...)
		forgeLine := "Matched named blueprints: " + strings.Join(forgeNames, ",")
		if strings.TrimSpace(result.RecommendedForges) != "" {
			result.RecommendedForges = strings.TrimSpace(result.RecommendedForges) + "\n" + forgeLine
		} else {
			result.RecommendedForges = forgeLine
		}
	}

	if skillNames := matches.SkillNames(); len(skillNames) > 0 {
		result.MatchedSkillNames = mergeCapabilityNames(result.MatchedSkillNames, skillNames...)
	}

	matchSection := matches.RenderMarkdown("### Referenced Capabilities")
	if matchSection == "" {
		return
	}
	if strings.TrimSpace(result.ContextEnrichment) != "" {
		result.ContextEnrichment = strings.TrimSpace(result.ContextEnrichment) + "\n\n" + matchSection
	} else {
		result.ContextEnrichment = matchSection
	}
}

func PopulateExtraCapabilitiesFromCapabilityMatches(r aicommon.AIInvokeRuntime, loop *ReActLoop, matches *CapabilityNameMatchResult) {
	if r == nil || loop == nil || matches == nil || !matches.HasMatches() {
		return
	}

	ecm := loop.GetExtraCapabilities()
	if ecm == nil {
		return
	}

	cfg := r.GetConfig()
	if cfg == nil {
		return
	}

	if toolNames := matches.ToolNames(); len(toolNames) > 0 {
		toolMgr := cfg.GetAiToolManager()
		if toolMgr != nil {
			for _, name := range toolNames {
				tool, err := toolMgr.GetToolByName(name)
				if err != nil {
					continue
				}
				ecm.AddTools(tool)
			}
		}
	}

	if forgeNames := matches.ForgeNames(); len(forgeNames) > 0 {
		type forgeManagerProvider interface {
			GetAIForgeManager() aicommon.AIForgeFactory
		}
		if provider, ok := cfg.(forgeManagerProvider); ok {
			forgeMgr := provider.GetAIForgeManager()
			if forgeMgr != nil {
				for _, name := range forgeNames {
					forge, err := forgeMgr.GetAIForge(name)
					if err != nil {
						continue
					}
					ecm.AddForges(ExtraForgeInfo{
						Name:        forge.ForgeName,
						VerboseName: forge.ForgeVerboseName,
						Description: forge.Description,
					})
				}
			}
		}
	}

	if skillNames := matches.SkillNames(); len(skillNames) > 0 {
		for _, skill := range matches.MatchedSkills {
			if skill == nil {
				continue
			}
			ecm.AddSkills(ExtraSkillInfo{
				Name:        skill.Name,
				Description: skill.Description,
			})
		}
	}
}
