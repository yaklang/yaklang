package aid

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
)

const (
	planFactsPersistentKey    = "plan_facts"
	planDocumentPersistentKey = "plan_document"
)

var (
	planFactsAITags    = []string{"FACTS", "PLAN_FACTS"}
	planEvidenceAITags = []string{"EVIDENCE", "PLAN_EVIDENCE"}
	planDocumentAITags = []string{"DOCUMENT", "PLAN_DOCUMENT"}
	planContextGapRE   = regexp.MustCompile(`\n{3,}`)
)

type discoveredAITagBlock struct {
	TagName string
	Nonce   string
	Start   int
	End     int
}

func extractPlanFactsFromText(content string) string {
	return extractPlanContextFromText(content, planFactsAITags...)
}

func extractPlanDocumentFromText(content string) string {
	return extractPlanContextFromText(content, planDocumentAITags...)
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
	allTags := make([]string, 0, len(planFactsAITags)+len(planDocumentAITags)+len(planEvidenceAITags))
	allTags = append(allTags, planFactsAITags...)
	allTags = append(allTags, planDocumentAITags...)
	allTags = append(allTags, planEvidenceAITags...)
	blocks := discoverAITagBlocks(content, allTags...)
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

func getTaskPlanEvidence(task *AiTask) string {
	if task == nil || task.Coordinator == nil || task.Coordinator.Config == nil {
		return ""
	}
	return task.Coordinator.GetSessionEvidenceRendered()
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

func buildVerificationCarryoverEvidenceOps(task *AiTask, reasoning string) []aicommon.EvidenceOperation {
	var ops []aicommon.EvidenceOperation
	taskLabel := formatTaskPlanEvidenceLabel(task)

	reasoning = strings.TrimSpace(reasoning)
	if reasoning != "" {
		ops = append(ops, aicommon.EvidenceOperation{
			Op:      "add",
			ID:      fmt.Sprintf("verify-%s", task.GetIndex()),
			Content: fmt.Sprintf("[%s] 核实: %s", taskLabel, reasoning),
		})
	}
	return ops
}

func buildSummaryEvidenceOps(task *AiTask, summary string) []aicommon.EvidenceOperation {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	taskLabel := formatTaskPlanEvidenceLabel(task)
	return []aicommon.EvidenceOperation{
		{
			Op:      "add",
			ID:      fmt.Sprintf("summary-%s", task.GetIndex()),
			Content: fmt.Sprintf("[%s] 总结: %s", taskLabel, summary),
		},
	}
}
