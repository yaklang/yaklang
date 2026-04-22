package reactloops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	envAIWorkspaceDebugPrimary   = "YAKIT_AI_WORKSPACE_DEBUG"
	envAIWorkspaceDebugSecondary = "AI_WORKSPACE_DEBUG"
)

func IsAIWorkspaceDebugEnabled() bool {
	for _, envKey := range []string{envAIWorkspaceDebugPrimary, envAIWorkspaceDebugSecondary} {
		raw := strings.TrimSpace(os.Getenv(envKey))
		if raw == "" {
			continue
		}
		return utils.InterfaceToBoolean(raw)
	}
	return false
}

func getAIWorkspaceDebugComponentDir(cfg aicommon.AICallerConfigIf, component string) string {
	if cfg == nil || !IsAIWorkspaceDebugEnabled() {
		return ""
	}

	workdir := cfg.GetOrCreateWorkDir()
	if workdir == "" {
		return ""
	}

	component = sanitizeAIWorkspaceDebugName(component)
	if component == "" {
		component = "misc"
	}

	dir := filepath.Join(workdir, "debug", component)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Warnf("failed to create ai workspace debug dir %s: %v", dir, err)
		return ""
	}
	return dir
}

func writeAIWorkspaceDebugMarkdown(cfg aicommon.AICallerConfigIf, component, filenamePrefix, markdown string) string {
	if strings.TrimSpace(markdown) == "" {
		return ""
	}

	dir := getAIWorkspaceDebugComponentDir(cfg, component)
	if dir == "" {
		return ""
	}

	filenamePrefix = sanitizeAIWorkspaceDebugName(filenamePrefix)
	if filenamePrefix == "" {
		filenamePrefix = component
	}

	filename := fmt.Sprintf("%s_%s_%d.md", filenamePrefix, time.Now().Format("20060102_150405"), time.Now().UnixNano())
	filePath := filepath.Join(dir, filename)
	if err := os.WriteFile(filePath, []byte(markdown), 0o644); err != nil {
		log.Warnf("failed to write ai workspace debug markdown %s: %v", filePath, err)
		return ""
	}
	return filePath
}

func sanitizeAIWorkspaceDebugName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		" ", "_",
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	name = replacer.Replace(name)
	name = strings.Trim(name, "_")
	return name
}

func appendWorkspaceDebugSection(buf *strings.Builder, title, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	buf.WriteString("## ")
	buf.WriteString(title)
	buf.WriteString("\n\n")
	buf.WriteString(content)
	buf.WriteString("\n\n")
}

func writeIntentRecognitionDebugMarkdown(r aicommon.AIInvokeRuntime, loop *ReActLoop, result *DeepIntentResult) string {
	if r == nil || loop == nil || result == nil {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("# Intent Recognition Debug\n\n")
	buf.WriteString(fmt.Sprintf("- Generated At: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	buf.WriteString(fmt.Sprintf("- Loop Name: %s\n\n", loop.loopName))

	appendWorkspaceDebugSection(&buf, "Intent Analysis", result.IntentAnalysis)
	appendWorkspaceDebugSection(&buf, "Recommended Tools", result.RecommendedTools)
	appendWorkspaceDebugSection(&buf, "Recommended Forges", result.RecommendedForges)
	appendWorkspaceDebugSection(&buf, "Context Enrichment", result.ContextEnrichment)

	appendWorkspaceDebugSection(&buf, "Matched Tool Names", result.MatchedToolNames)
	appendWorkspaceDebugSection(&buf, "Matched Forge Names", result.MatchedForgeNames)
	appendWorkspaceDebugSection(&buf, "Matched Skill Names", result.MatchedSkillNames)

	appendWorkspaceDebugSection(&buf, "Search Results", loop.Get("search_results"))
	appendWorkspaceDebugSection(&buf, "Matched Capability Details (JSON)", loop.Get("matched_capabilities_details"))

	task := loop.GetCurrentTask()
	if task != nil && task.GetTaskRetrievalInfo() != nil {
		info := task.GetTaskRetrievalInfo()
		var retrieval strings.Builder
		if strings.TrimSpace(info.Target) != "" {
			retrieval.WriteString("Target: ")
			retrieval.WriteString(strings.TrimSpace(info.Target))
			retrieval.WriteString("\n")
		}
		if len(info.Tags) > 0 {
			retrieval.WriteString("Tags: ")
			retrieval.WriteString(strings.Join(info.Tags, ", "))
			retrieval.WriteString("\n")
		}
		if len(info.Questions) > 0 {
			retrieval.WriteString("Questions:\n")
			for _, question := range info.Questions {
				retrieval.WriteString("- ")
				retrieval.WriteString(question)
				retrieval.WriteString("\n")
			}
		}
		appendWorkspaceDebugSection(&buf, "Task Retrieval Info", retrieval.String())
	}

	return writeAIWorkspaceDebugMarkdown(r.GetConfig(), "intent", "intent", buf.String())
}

func writePerceptionDebugMarkdown(loop *ReActLoop, state *PerceptionState, input CapabilitySearchInput, result *CapabilitySearchResult, searchErr error) string {
	if loop == nil || state == nil {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("# Perception Debug\n\n")
	buf.WriteString(fmt.Sprintf("- Generated At: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	buf.WriteString(fmt.Sprintf("- Loop Name: %s\n", loop.loopName))
	buf.WriteString(fmt.Sprintf("- Epoch: %d\n", state.Epoch))
	buf.WriteString(fmt.Sprintf("- Trigger: %s\n", state.LastTrigger))
	buf.WriteString(fmt.Sprintf("- Changed: %v\n", state.Changed))
	buf.WriteString(fmt.Sprintf("- Confidence: %.4f\n\n", state.ConfidenceLevel))

	appendWorkspaceDebugSection(&buf, "Summary", state.OneLinerSummary)
	appendWorkspaceDebugSection(&buf, "Topics", strings.Join(state.Topics, ", "))
	appendWorkspaceDebugSection(&buf, "Keywords", strings.Join(state.Keywords, ", "))

	var queryInfo strings.Builder
	if strings.TrimSpace(input.Query) != "" {
		queryInfo.WriteString("Query: ")
		queryInfo.WriteString(strings.TrimSpace(input.Query))
		queryInfo.WriteString("\n")
	}
	if len(input.Queries) > 0 {
		queryInfo.WriteString("Queries:\n")
		for _, query := range input.Queries {
			queryInfo.WriteString("- ")
			queryInfo.WriteString(query)
			queryInfo.WriteString("\n")
		}
	}
	appendWorkspaceDebugSection(&buf, "Capability Search Input", queryInfo.String())

	if searchErr != nil {
		appendWorkspaceDebugSection(&buf, "Capability Search Error", searchErr.Error())
	}
	if result != nil {
		appendWorkspaceDebugSection(&buf, "Capability Search Results", result.SearchResultsMarkdown)
		appendWorkspaceDebugSection(&buf, "Capability Context Enrichment", result.ContextEnrichment)
		appendWorkspaceDebugSection(&buf, "Matched Tool Names", strings.Join(result.MatchedToolNames, ", "))
		appendWorkspaceDebugSection(&buf, "Matched Forge Names", strings.Join(result.MatchedForgeNames, ", "))
		appendWorkspaceDebugSection(&buf, "Matched Skill Names", strings.Join(result.MatchedSkillNames, ", "))
		appendWorkspaceDebugSection(&buf, "Matched Focus Modes", strings.Join(result.MatchedFocusModeNames, ", "))
		appendWorkspaceDebugSection(&buf, "Recommended Capabilities", strings.Join(result.RecommendedCapabilities, ", "))
	}

	return writeAIWorkspaceDebugMarkdown(loop.GetConfig(), "perception", fmt.Sprintf("perception_epoch_%d", state.Epoch), buf.String())
}
