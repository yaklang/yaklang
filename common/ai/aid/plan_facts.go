package aid

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	planFactsPersistentKey    = "plan_facts"
	planEvidencePersistentKey = "plan_evidence"
	planEvidenceTokenBudget   = 15000
)

var (
	planFactsAITags    = []string{"FACTS", "PLAN_FACTS"}
	planEvidenceAITags = []string{"EVIDENCE", "PLAN_EVIDENCE"}
	planContextGapRE   = regexp.MustCompile(`\n{3,}`)
)

type discoveredAITagBlock struct {
	TagName string
	Nonce   string
	Start   int
	End     int
}

func buildFactsBlock(facts string) string {
	return buildPlanContextBlock("FACTS", facts)
}

func buildEvidenceBlock(evidence string) string {
	return buildPlanContextBlock("EVIDENCE", evidence)
}

func buildPlanContextBlock(tag string, content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	nonce := utils.RandStringBytes(6)
	return fmt.Sprintf("<|%s_%s|>\n%s\n<|%s_END_%s|>", tag, nonce, content, tag, nonce)
}

func prependPlanFactsToRenderedPlan(base string, facts string) string {
	return prependPlanContextDocsToRenderedPlan(base, facts, "")
}

func prependPlanContextDocsToRenderedPlan(base string, facts string, evidence string) string {
	facts = strings.TrimSpace(facts)
	evidence = strings.TrimSpace(evidence)
	if facts == "" && evidence == "" {
		return base
	}
	blocks := make([]string, 0, 2)
	if facts != "" {
		blocks = append(blocks, buildFactsBlock(facts))
	}
	if evidence != "" {
		blocks = append(blocks, buildEvidenceBlock(evidence))
	}
	joinedBlocks := strings.Join(blocks, "\n\n")
	if strings.TrimSpace(base) == "" {
		return joinedBlocks
	}
	return joinedBlocks + "\n\n" + base
}

func extractPlanFactsFromText(content string) string {
	return extractPlanContextFromText(content, planFactsAITags...)
}

func extractPlanEvidenceFromText(content string) string {
	return extractPlanContextFromText(content, planEvidenceAITags...)
}

func extractPlanContextFromText(content string, tagNames ...string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	blocks := discoverAITagBlocks(content, tagNames...)
	if len(blocks) == 0 {
		return ""
	}

	results := make([]string, len(blocks))
	options := make([]aitag.ParseOption, 0, len(blocks))
	var mu sync.Mutex
	for index, block := range blocks {
		index := index
		block := block
		options = append(options, aitag.WithCallback(block.TagName, block.Nonce, func(reader io.Reader) {
			contentBytes, err := io.ReadAll(reader)
			if err != nil {
				return
			}
			mu.Lock()
			results[index] = strings.TrimSpace(string(contentBytes))
			mu.Unlock()
		}))
	}
	if err := aitag.Parse(strings.NewReader(content), options...); err != nil {
		return ""
	}
	for _, result := range results {
		if result != "" {
			return result
		}
	}
	return ""
}

func stripPlanContextBlocks(content string) string {
	content = strings.TrimSpace(content)
	blocks := discoverAITagBlocks(content, append(planFactsAITags, planEvidenceAITags...)...)
	if len(blocks) == 0 {
		return content
	}

	var builder strings.Builder
	last := 0
	for _, block := range blocks {
		if block.Start > last {
			builder.WriteString(content[last:block.Start])
		}
		if block.End > last {
			last = block.End
		}
	}
	if last < len(content) {
		builder.WriteString(content[last:])
	}
	cleaned := strings.TrimSpace(builder.String())
	return planContextGapRE.ReplaceAllString(cleaned, "\n\n")
}

func discoverAITagBlocks(content string, tagNames ...string) []discoveredAITagBlock {
	if content == "" || len(tagNames) == 0 {
		return nil
	}
	allowedTags := make(map[string]struct{}, len(tagNames))
	for _, tagName := range tagNames {
		if tagName == "" {
			continue
		}
		allowedTags[tagName] = struct{}{}
	}
	if len(allowedTags) == 0 {
		return nil
	}

	blocks := make([]discoveredAITagBlock, 0, 4)
	for offset := 0; offset < len(content); {
		startOffset := strings.Index(content[offset:], "<|")
		if startOffset < 0 {
			break
		}
		start := offset + startOffset
		tagCloseOffset := strings.Index(content[start:], "|>")
		if tagCloseOffset < 0 {
			break
		}
		tagClose := start + tagCloseOffset + 2
		tagName, nonce, ok := parseAITagStartToken(content[start+2 : tagClose-2])
		if !ok {
			offset = tagClose
			continue
		}
		if _, exists := allowedTags[tagName]; !exists {
			offset = tagClose
			continue
		}

		endTag := fmt.Sprintf("<|%s_END_%s|>", tagName, nonce)
		endOffset := strings.Index(content[tagClose:], endTag)
		if endOffset < 0 {
			offset = tagClose
			continue
		}
		end := tagClose + endOffset + len(endTag)
		blocks = append(blocks, discoveredAITagBlock{
			TagName: tagName,
			Nonce:   nonce,
			Start:   start,
			End:     end,
		})
		offset = end
	}
	return blocks
}

func parseAITagStartToken(token string) (string, string, bool) {
	if token == "" || strings.Contains(token, "_END_") {
		return "", "", false
	}
	underscore := strings.LastIndex(token, "_")
	if underscore <= 0 || underscore >= len(token)-1 {
		return "", "", false
	}
	tagName := token[:underscore]
	nonce := token[underscore+1:]
	for _, ch := range tagName {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
			return "", "", false
		}
	}
	return tagName, nonce, true
}

func getTaskPlanFacts(task *AiTask) string {
	return getTaskPlanPersistentMarkdown(task, planFactsPersistentKey, extractPlanFactsFromText)
}

func getTaskPlanEvidence(task *AiTask) string {
	return getTaskPlanPersistentMarkdown(task, planEvidencePersistentKey, extractPlanEvidenceFromText)
}

func getTaskPlanPersistentMarkdown(task *AiTask, key string, extractor func(string) string) string {
	if task == nil {
		return ""
	}
	if task.Coordinator != nil && task.Coordinator.ContextProvider != nil {
		if content, ok := task.Coordinator.ContextProvider.GetPersistentData(key); ok {
			content = strings.TrimSpace(content)
			if content != "" {
				return content
			}
		}
	}
	root := task
	for root.ParentTask != nil {
		root = root.ParentTask
	}
	if root.AIStatefulTaskBase != nil {
		if content := extractor(root.AIStatefulTaskBase.GetUserInput()); content != "" {
			return content
		}
	}
	return ""
}

func mergePlanContextDocuments(existing string, incoming string) string {
	existing = strings.TrimSpace(existing)
	incoming = strings.TrimSpace(incoming)
	if incoming == "" {
		return existing
	}
	if existing == "" {
		return incoming
	}
	if strings.Contains(existing, incoming) {
		return existing
	}
	if strings.Contains(incoming, existing) {
		return incoming
	}
	return strings.TrimSpace(existing + "\n\n" + incoming)
}

func appendTaskPlanEvidence(task *AiTask, incoming string) (string, bool) {
	incoming = aicommon.NormalizeConcreteEvidenceMarkdown(incoming)
	if task == nil || incoming == "" {
		return getTaskPlanEvidence(task), false
	}
	existing := getTaskPlanEvidence(task)
	merged := mergePlanContextDocuments(existing, incoming)
	merged = strings.TrimSpace(aicommon.ShrinkTextBlockByTokens(merged, planEvidenceTokenBudget))
	if merged == existing {
		return merged, false
	}
	if task.Coordinator != nil && task.Coordinator.ContextProvider != nil {
		task.Coordinator.ContextProvider.SetPersistentData(planEvidencePersistentKey, merged)
	}
	syncRootTaskPlanContextDocs(task)
	return merged, true
}

func buildTaskPlanVerificationCarryoverMarkdown(task *AiTask, reasoning string, outputFiles []string) string {
	sections := make([]string, 0, 3)
	taskLabel := formatTaskPlanEvidenceLabel(task)
	reasoning = strings.TrimSpace(reasoning)

	if reasoning != "" {
		parts := []string{fmt.Sprintf("## %s 核实结果", taskLabel)}
		parts = append(parts, "### 判定", reasoning)
		sections = append(sections, strings.TrimSpace(strings.Join(parts, "\n\n")))
	}

	normalizedFiles := normalizeTaskPlanOutputFiles(outputFiles)
	if len(normalizedFiles) > 0 {
		lines := make([]string, 0, len(normalizedFiles)+1)
		lines = append(lines, fmt.Sprintf("## %s 交付文件", taskLabel))
		for _, filePath := range normalizedFiles {
			lines = append(lines, "- "+filePath)
		}
		sections = append(sections, strings.TrimSpace(strings.Join(lines, "\n")))
	}

	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func buildTaskPlanSummaryCarryoverMarkdown(task *AiTask, summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("## %s 任务总结\n\n%s", formatTaskPlanEvidenceLabel(task), summary))
}

func formatTaskPlanEvidenceLabel(task *AiTask) string {
	if task == nil {
		return "当前任务"
	}
	index := strings.TrimSpace(task.GetIndex())
	name := strings.TrimSpace(task.GetName())
	if index == "" && name == "" {
		return "当前任务"
	}
	if index == "" {
		return name
	}
	if name == "" {
		return "子任务 " + index
	}
	return fmt.Sprintf("子任务 %s %s", index, name)
}

func normalizeTaskPlanOutputFiles(outputFiles []string) []string {
	if len(outputFiles) == 0 {
		return nil
	}
	result := make([]string, 0, len(outputFiles))
	seen := make(map[string]struct{}, len(outputFiles))
	for _, filePath := range outputFiles {
		normalizedPath := sanitizeTaskPlanOutputFilePath(filePath)
		if normalizedPath == "" {
			continue
		}
		if _, exists := seen[normalizedPath]; exists {
			continue
		}
		seen[normalizedPath] = struct{}{}
		result = append(result, normalizedPath)
	}
	return result
}

func sanitizeTaskPlanOutputFilePath(filePath string) string {
	cleaned := strings.TrimSpace(filePath)
	if cleaned == "" {
		return ""
	}
	cleaned = strings.NewReplacer("\r", "", "\n", "", "\t", " ").Replace(cleaned)
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return ""
	}
	base := filepath.Base(cleaned)
	if strings.HasPrefix(base, "ai_bash_script_") && strings.HasSuffix(base, ".sh") {
		return ""
	}
	return cleaned
}

func syncRootTaskPlanContextDocs(task *AiTask) {
	if task == nil {
		return
	}
	root := task
	for root.ParentTask != nil {
		root = root.ParentTask
	}
	if root.AIStatefulTaskBase == nil {
		return
	}
	base := stripPlanContextBlocks(root.AIStatefulTaskBase.GetUserInput())
	root.SetUserInput(prependPlanContextDocsToRenderedPlan(base, getTaskPlanFacts(root), getTaskPlanEvidence(root)))
}
