package reactloops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const CapabilityCatalogChunkSize = 30 * 1024

type CapabilityDetail struct {
	CapabilityName string `json:"capability_name"`
	CapabilityType string `json:"capability_type"`
	Description    string `json:"description"`
}

type CapabilitySearchInput struct {
	Query                   string
	Queries                 []string
	RecommendedCapabilities []string
	IncludeCatalogMatch     bool
	Limit                   int
}

type CapabilitySearchResult struct {
	SearchResultsMarkdown string
	ContextEnrichment     string

	MatchedToolNames      []string
	MatchedForgeNames     []string
	MatchedSkillNames     []string
	MatchedFocusModeNames []string

	RecommendedCapabilities []string
	CatalogMatchedNames     []string
	Details                 []CapabilityDetail
}

var capabilityTypeUsageGuides = map[string]string{
	"tool":       "通过 `require_tool` 调用指定工具执行任务。/ Use `require_tool` to invoke the tool.",
	"forge":      "通过 `require_ai_blueprint` 调用蓝图，由蓝图系统负责自动化执行编排。/ Use `require_ai_blueprint` to execute the blueprint workflow.",
	"skill":      "技能会被自动加载到上下文中，提供特定领域的知识和方法指引。/ Skills are auto-loaded into context.",
	"focus_mode": "通过 `enter_focus_mode` 进入专注模式，在独立的执行环境中完成特定任务。/ Use `enter_focus_mode` to enter focus mode.",
}

var capabilityTypeLabels = map[string]string{
	"tool":       "Tools (工具)",
	"forge":      "Forges / Blueprints (AI 蓝图)",
	"skill":      "Skills (技能)",
	"focus_mode": "Focus Modes (专注模式)",
}

var capabilityTypeOrder = []string{"tool", "forge", "skill", "focus_mode"}

func SearchCapabilities(r aicommon.AIInvokeRuntime, loop *ReActLoop, input CapabilitySearchInput) (*CapabilitySearchResult, error) {
	queries := normalizeCapabilityStrings(append([]string{input.Query}, input.Queries...))
	if len(queries) == 0 {
		return &CapabilitySearchResult{}, nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}

	result := &CapabilitySearchResult{}
	var markdown strings.Builder
	markdown.WriteString(fmt.Sprintf("## Capability Search Results for: %s\n\n", strings.Join(queries, " | ")))

	for _, query := range queries {
		keywords := normalizeCapabilityStrings(strings.Fields(query))
		if len(keywords) == 0 {
			continue
		}
		searchToolsAndForges(query, keywords, limit, result, &markdown)
		searchSkillsFromLoader(r, query, limit, result, &markdown)
		searchFocusModes(query, result, &markdown)
	}

	if input.IncludeCatalogMatch {
		catalog := BuildCapabilityCatalog(r)
		matched := MatchIdentifiersFromCapabilityCatalog(r, catalog, strings.Join(queries, " "))
		if len(matched) > 0 {
			verified := VerifyCapabilityIdentifiers(loop, matched)
			result.CatalogMatchedNames = verified
			if len(verified) > 0 {
				markdown.WriteString("### Pre-matched from Capability Catalog\n")
				markdown.WriteString(strings.Join(verified, ","))
				markdown.WriteString("\n\n")
			}
		}
	}

	result.MatchedToolNames = normalizeCapabilityStrings(result.MatchedToolNames)
	result.MatchedForgeNames = normalizeCapabilityStrings(result.MatchedForgeNames)
	result.MatchedSkillNames = normalizeCapabilityStrings(result.MatchedSkillNames)
	result.MatchedFocusModeNames = normalizeCapabilityStrings(result.MatchedFocusModeNames)
	result.Details = dedupeCapabilityDetails(result.Details)

	recommended := normalizeCapabilityStrings(append(input.RecommendedCapabilities, result.CatalogMatchedNames...))
	recommended = VerifyCapabilityIdentifiers(loop, recommended)
	if len(recommended) == 0 {
		recommended = normalizeCapabilityStrings(append(
			append(append([]string{}, result.MatchedToolNames...), result.MatchedForgeNames...),
			append(result.MatchedSkillNames, result.MatchedFocusModeNames...)...,
		))
	}
	result.RecommendedCapabilities = recommended

	recSet := make(map[string]bool, len(recommended))
	for _, name := range recommended {
		recSet[name] = true
	}
	result.ContextEnrichment = BuildCapabilityEnrichmentMarkdown(result.Details, recSet)
	result.SearchResultsMarkdown = strings.TrimSpace(markdown.String())
	return result, nil
}

func ApplyCapabilitySearchResult(r aicommon.AIInvokeRuntime, loop *ReActLoop, result *CapabilitySearchResult) {
	if r == nil || loop == nil || result == nil {
		return
	}
	if result.SearchResultsMarkdown != "" {
		loop.Set("capability_search_results", result.SearchResultsMarkdown)
	}
	if result.ContextEnrichment != "" {
		loop.Set("capability_context_enrichment", result.ContextEnrichment)
	}
	if len(result.MatchedToolNames) > 0 {
		loop.Set("matched_tool_names", strings.Join(result.MatchedToolNames, ","))
	}
	if len(result.MatchedForgeNames) > 0 {
		loop.Set("matched_forge_names", strings.Join(result.MatchedForgeNames, ","))
	}
	if len(result.MatchedSkillNames) > 0 {
		loop.Set("matched_skill_names", strings.Join(result.MatchedSkillNames, ","))
	}
	if len(result.MatchedFocusModeNames) > 0 {
		loop.Set("matched_loop_names", strings.Join(result.MatchedFocusModeNames, ","))
	}
	if len(result.RecommendedCapabilities) > 0 {
		loop.Set("recommended_capabilities", strings.Join(result.RecommendedCapabilities, ","))
		PreloadSingleRecommendedTool(loop, result.RecommendedCapabilities)
	}
	PopulateExtraCapabilitiesFromCapabilitySearchResult(r, loop, result)
}

func PopulateExtraCapabilitiesFromCapabilitySearchResult(r aicommon.AIInvokeRuntime, loop *ReActLoop, result *CapabilitySearchResult) {
	if r == nil || loop == nil || result == nil {
		return
	}
	ecm := loop.GetExtraCapabilities()
	if ecm == nil {
		return
	}

	cfg := r.GetConfig()
	if len(result.MatchedToolNames) > 0 {
		toolMgr := cfg.GetAiToolManager()
		if toolMgr != nil {
			for _, name := range result.MatchedToolNames {
				tool, err := toolMgr.GetToolByName(name)
				if err != nil {
					log.Debugf("capability search: skip tool %q: %v", name, err)
					continue
				}
				ecm.AddTools(tool)
			}
		}
	}

	if len(result.MatchedForgeNames) > 0 {
		type forgeManagerProvider interface {
			GetAIForgeManager() aicommon.AIForgeFactory
		}
		if provider, ok := cfg.(forgeManagerProvider); ok {
			forgeMgr := provider.GetAIForgeManager()
			if forgeMgr != nil {
				for _, name := range result.MatchedForgeNames {
					forge, err := forgeMgr.GetAIForge(name)
					if err != nil {
						log.Debugf("capability search: skip forge %q: %v", name, err)
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

	if len(result.MatchedSkillNames) > 0 {
		type skillLoaderProvider interface {
			GetSkillLoader() aiskillloader.SkillLoader
		}
		if provider, ok := cfg.(skillLoaderProvider); ok {
			skillLoader := provider.GetSkillLoader()
			if skillLoader != nil && skillLoader.HasSkills() {
				for _, name := range result.MatchedSkillNames {
					meta, err := aiskillloader.LookupSkillMeta(skillLoader, name)
					if err != nil || meta == nil {
						log.Debugf("capability search: skip skill %q: %v", name, err)
						continue
					}
					ecm.AddSkills(ExtraSkillInfo{Name: meta.Name, Description: meta.Description})
				}
			}
		}
	}

	for _, name := range result.MatchedFocusModeNames {
		if meta, ok := GetLoopMetadata(name); ok {
			ecm.AddFocusModes(ExtraFocusModeInfo{Name: meta.Name, Description: meta.Description})
		}
	}
}

func BuildCapabilityCatalog(r aicommon.AIInvokeRuntime) string {
	var sb strings.Builder

	db := consts.GetGormProfileDatabase()
	if db != nil {
		tools, err := yakit.SearchAIYakTool(db, "")
		if err != nil {
			log.Warnf("capability catalog: failed to load tools: %v", err)
		} else {
			for _, tool := range tools {
				name := tool.VerboseName
				if name == "" {
					name = tool.Name
				}
				desc := utils.ShrinkString(tool.Description, 120)
				line := fmt.Sprintf("[tool:%s]: %s - %s", tool.Name, name, desc)
				if tool.Keywords != "" {
					line += fmt.Sprintf(". keywords: %s", utils.ShrinkString(tool.Keywords, 80))
				}
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}

		forges, err := yakit.GetAllAIForge(db)
		if err != nil {
			log.Warnf("capability catalog: failed to load forges: %v", err)
		} else {
			for _, forge := range forges {
				name := forge.ForgeVerboseName
				if name == "" {
					name = forge.ForgeName
				}
				desc := utils.ShrinkString(forge.Description, 120)
				line := fmt.Sprintf("[forge:%s]: %s - %s", forge.ForgeName, name, desc)
				if forge.ToolKeywords != "" {
					line += fmt.Sprintf(". keywords: %s", utils.ShrinkString(forge.ToolKeywords, 80))
				}
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
	}

	type skillLoaderProvider interface {
		GetSkillLoader() aiskillloader.SkillLoader
	}
	cfg := r.GetConfig()
	if provider, ok := cfg.(skillLoaderProvider); ok {
		skillLoader := provider.GetSkillLoader()
		if skillLoader != nil && skillLoader.HasSkills() {
			for _, meta := range skillLoader.AllSkillMetas() {
				desc := utils.ShrinkString(meta.Description, 120)
				sb.WriteString(fmt.Sprintf("[skill:%s]: %s - %s\n", meta.Name, meta.Name, desc))
			}
		}
	}

	for _, meta := range GetAllLoopMetadata() {
		if meta.IsHidden {
			continue
		}
		desc := utils.ShrinkString(meta.Description, 120)
		sb.WriteString(fmt.Sprintf("[focus_mode:%s]: %s - %s\n", meta.Name, meta.Name, desc))
	}
	return sb.String()
}

func MatchIdentifiersFromCapabilityCatalog(r aicommon.AIInvokeRuntime, catalog string, query string) []string {
	if catalog == "" || query == "" {
		return nil
	}
	chunks := SplitCapabilityCatalogIntoChunks(catalog, CapabilityCatalogChunkSize)
	if len(chunks) == 0 {
		return nil
	}
	ctx := r.GetConfig().GetContext()

	var mu sync.Mutex
	var allIdentifiers []string
	var wg sync.WaitGroup
	for index, chunk := range chunks {
		wg.Add(1)
		go func(chunkIndex int, chunkData string) {
			defer wg.Done()
			ids := matchCapabilityCatalogChunk(ctx, r, chunkData, query, chunkIndex)
			if len(ids) > 0 {
				mu.Lock()
				allIdentifiers = append(allIdentifiers, ids...)
				mu.Unlock()
			}
		}(index, chunk)
	}
	wg.Wait()
	return normalizeCapabilityStrings(allIdentifiers)
}

func SplitCapabilityCatalogIntoChunks(catalog string, maxChunkBytes int) []string {
	if len(catalog) <= maxChunkBytes {
		return []string{catalog}
	}
	lines := strings.Split(catalog, "\n")
	var chunks []string
	var current strings.Builder
	for _, line := range lines {
		if current.Len()+len(line)+1 > maxChunkBytes && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func VerifyCapabilityIdentifiers(loop *ReActLoop, identifiers []string) []string {
	if loop == nil {
		return normalizeCapabilityStrings(identifiers)
	}
	var verified []string
	for _, id := range normalizeCapabilityStrings(identifiers) {
		resolved := loop.ResolveIdentifier(id)
		if resolved.IsUnknown() {
			log.Infof("capability catalog: identifier %q not resolved, skipping", id)
			continue
		}
		verified = append(verified, id)
	}
	return verified
}

func BuildCapabilityEnrichmentMarkdown(details []CapabilityDetail, recommendedNames map[string]bool) string {
	if len(details) == 0 {
		return ""
	}
	grouped := make(map[string][]CapabilityDetail)
	for _, detail := range details {
		if len(recommendedNames) > 0 && !recommendedNames[detail.CapabilityName] {
			continue
		}
		grouped[detail.CapabilityType] = append(grouped[detail.CapabilityType], detail)
	}

	var md strings.Builder
	md.WriteString("### Recommended Capabilities / 推荐能力\n\n")
	hasContent := false
	for _, capType := range capabilityTypeOrder {
		caps := grouped[capType]
		if len(caps) == 0 {
			continue
		}
		hasContent = true
		label := capabilityTypeLabels[capType]
		if label == "" {
			label = capType
		}
		md.WriteString(fmt.Sprintf("#### %s\n", label))
		if guide, ok := capabilityTypeUsageGuides[capType]; ok {
			md.WriteString(guide)
			md.WriteString("\n\n")
		}
		for _, cap := range caps {
			md.WriteString(fmt.Sprintf("- **%s**: %s\n", cap.CapabilityName, cap.Description))
		}
		md.WriteString("\n")
	}
	if !hasContent {
		return ""
	}
	return md.String()
}

func MarshalCapabilityDetails(details []CapabilityDetail) string {
	if len(details) == 0 {
		return ""
	}
	data, err := json.Marshal(details)
	if err != nil {
		log.Warnf("capability search: failed to marshal capability details: %v", err)
		return ""
	}
	return string(data)
}

func ParseCapabilityDetails(jsonStr string) []CapabilityDetail {
	if jsonStr == "" {
		return nil
	}
	var details []CapabilityDetail
	if err := json.Unmarshal([]byte(jsonStr), &details); err != nil {
		log.Warnf("capability search: failed to parse capability details: %v", err)
		return nil
	}
	return details
}

func searchToolsAndForges(query string, keywords []string, limit int, result *CapabilitySearchResult, markdown *strings.Builder) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		markdown.WriteString("### Tools & Forges\nDatabase not available.\n\n")
		return
	}
	tools, err := yakit.SearchAIYakToolBM25(db, &yakit.AIYakToolFilter{Keywords: keywords}, limit, 0)
	if err != nil {
		log.Warnf("capability search: BM25 tool search failed: %v", err)
	} else if len(tools) > 0 {
		markdown.WriteString(fmt.Sprintf("### Matched Tools for: %s\n", query))
		for _, tool := range tools {
			name := tool.Name
			if tool.VerboseName != "" {
				name = tool.VerboseName + " (" + tool.Name + ")"
			}
			desc := utils.ShrinkString(tool.Description, 200)
			markdown.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
			result.MatchedToolNames = append(result.MatchedToolNames, tool.Name)
			result.Details = append(result.Details, CapabilityDetail{CapabilityName: tool.Name, CapabilityType: "tool", Description: desc})
		}
		markdown.WriteString("\n")
	}

	yakScripts, err := yakit.SearchYakScriptForAIBM25(db, &yakit.YakScriptForAIFilter{Keywords: keywords}, limit, 0)
	if err != nil {
		log.Warnf("capability search: yakit plugin search failed: %v", err)
	} else if len(yakScripts) > 0 {
		markdown.WriteString(fmt.Sprintf("### Matched Yakit Plugins for: %s\n", query))
		for _, script := range yakScripts {
			pluginType := strings.ToUpper(script.Type)
			desc := script.AIDesc
			if desc == "" {
				desc = script.Help
			}
			desc = utils.ShrinkString(desc, 200)
			markdown.WriteString(fmt.Sprintf("- **[%s] %s**: %s\n", pluginType, script.ScriptName, desc))
			result.MatchedToolNames = append(result.MatchedToolNames, script.ScriptName)
			result.Details = append(result.Details, CapabilityDetail{CapabilityName: script.ScriptName, CapabilityType: "yakit_plugin_" + script.Type, Description: desc})
		}
		markdown.WriteString("\n")
	}

	forges, err := yakit.SearchAIForgeBM25(db, &yakit.AIForgeSearchFilter{ForgeTypes: schema.RunnableForgeTypes(), Keywords: keywords}, limit, 0)
	if err != nil {
		log.Warnf("capability search: BM25 forge search failed: %v", err)
	} else if len(forges) > 0 {
		markdown.WriteString(fmt.Sprintf("### Matched AI Forges (Blueprints) for: %s\n", query))
		for _, forge := range forges {
			name := forge.ForgeName
			if forge.ForgeVerboseName != "" {
				name = forge.ForgeVerboseName + " (" + forge.ForgeName + ")"
			}
			desc := utils.ShrinkString(forge.Description, 200)
			markdown.WriteString(fmt.Sprintf("- **%s**: %s\n", name, desc))
			result.MatchedForgeNames = append(result.MatchedForgeNames, forge.ForgeName)
			result.Details = append(result.Details, CapabilityDetail{CapabilityName: forge.ForgeName, CapabilityType: "forge", Description: desc})
		}
		markdown.WriteString("\n")
	}
}

func searchSkillsFromLoader(r aicommon.AIInvokeRuntime, query string, limit int, result *CapabilitySearchResult, markdown *strings.Builder) {
	type skillLoaderProvider interface {
		GetSkillLoader() aiskillloader.SkillLoader
	}
	cfg := r.GetConfig()
	provider, ok := cfg.(skillLoaderProvider)
	if !ok {
		return
	}
	skillLoader := provider.GetSkillLoader()
	if skillLoader == nil || !skillLoader.HasSkills() {
		return
	}
	matchedSkills, err := aiskillloader.SearchSkillMetas(skillLoader, query, limit)
	if err != nil {
		log.Warnf("capability search: skill search failed: %v", err)
		return
	}
	if len(matchedSkills) == 0 {
		return
	}
	markdown.WriteString(fmt.Sprintf("### Matched Skills for: %s\n", query))
	for _, skill := range matchedSkills {
		desc := utils.ShrinkString(skill.Description, 200)
		markdown.WriteString(fmt.Sprintf("- **%s**: %s\n", skill.Name, desc))
		result.MatchedSkillNames = append(result.MatchedSkillNames, skill.Name)
		result.Details = append(result.Details, CapabilityDetail{CapabilityName: skill.Name, CapabilityType: "skill", Description: desc})
	}
	markdown.WriteString("\n")
}

func searchFocusModes(query string, result *CapabilitySearchResult, markdown *strings.Builder) {
	matched := searchLoopMetadata(query)
	if len(matched) == 0 {
		return
	}
	markdown.WriteString(fmt.Sprintf("### Matched Focus Modes for: %s\n", query))
	for _, meta := range matched {
		markdown.WriteString(fmt.Sprintf("- **%s**: %s\n", meta.Name, meta.Description))
		result.MatchedFocusModeNames = append(result.MatchedFocusModeNames, meta.Name)
		result.Details = append(result.Details, CapabilityDetail{CapabilityName: meta.Name, CapabilityType: "focus_mode", Description: meta.Description})
	}
	markdown.WriteString("\n")
}

func searchLoopMetadata(query string) []*LoopMetadata {
	allMeta := GetAllLoopMetadata()
	queryLower := strings.ToLower(query)
	queryTokens := strings.Fields(queryLower)
	var matched []*LoopMetadata
	for _, meta := range allMeta {
		if meta.IsHidden {
			continue
		}
		searchText := strings.ToLower(meta.Name + " " + meta.Description + " " + meta.UsagePrompt)
		if strings.Contains(searchText, queryLower) {
			matched = append(matched, meta)
			continue
		}
		if len(queryTokens) > 1 {
			meaningfulTokens := 0
			matchCount := 0
			for _, token := range queryTokens {
				if len(token) < 2 {
					continue
				}
				meaningfulTokens++
				if strings.Contains(searchText, token) {
					matchCount++
				}
			}
			if meaningfulTokens > 0 && matchCount > 0 && matchCount >= (meaningfulTokens+1)/2 {
				matched = append(matched, meta)
			}
		}
	}
	return matched
}

func matchCapabilityCatalogChunk(ctx context.Context, r aicommon.AIInvokeRuntime, chunkData string, query string, chunkIdx int) []string {
	nonce := utils.RandStringBytes(6)
	prompt := fmt.Sprintf(`<|INSTRUCTION_%s|>
You are a capability matcher. Given a user query and a catalog of available capabilities,
select ALL capabilities that are relevant to the user's intent or scenario.

CRITICAL RULES:
- You MUST ONLY select identifiers that appear in the catalog below. Do NOT invent or fabricate any identifier.
- If the user's input directly contains a capability identifier, that identifier MUST be included.
- Consider both Chinese and English meanings when matching.
- Return ONLY the identifier part (the text after the type prefix, e.g., "web_search" from "[tool:web_search]").
<|INSTRUCTION_END_%s|>

<|USER_QUERY_%s|>
%s
<|USER_QUERY_END_%s|>

<|CAPABILITY_CATALOG_%s|>
%s
<|CAPABILITY_CATALOG_END_%s|>`, nonce, nonce, nonce, query, nonce, nonce, chunkData, nonce)

	schema := []aitool.ToolOption{
		aitool.WithStringArrayParamEx("matched_identifiers", []aitool.PropertyOption{
			aitool.WithParam_Description("List of matched capability identifiers from the catalog. Only include identifiers that actually exist in the catalog."),
			aitool.WithParam_Required(true),
		}),
	}
	forgeResult, err := r.InvokeSpeedPriorityLiteForge(ctx, "capability-catalog-match", prompt, schema)
	if err != nil {
		log.Warnf("capability catalog match chunk %d failed: %v", chunkIdx, err)
		return nil
	}
	if forgeResult == nil {
		return nil
	}
	return forgeResult.GetStringSlice("matched_identifiers")
}

func normalizeCapabilityStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func dedupeCapabilityDetails(details []CapabilityDetail) []CapabilityDetail {
	if len(details) == 0 {
		return nil
	}
	result := make([]CapabilityDetail, 0, len(details))
	seen := make(map[string]struct{}, len(details))
	for _, detail := range details {
		key := detail.CapabilityType + ":" + detail.CapabilityName
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, detail)
	}
	return result
}
