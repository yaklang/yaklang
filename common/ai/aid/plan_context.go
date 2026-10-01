package aid

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

var planContextGapRE = regexp.MustCompile(`\n{3,}`)

type discoveredAITagBlock struct {
	Start int
	End   int
}

func stripPlanContextBlocks(content string) string {
	content = strings.TrimSpace(content)
	// Legacy task inputs may contain old plan context blocks. Strip those copies;
	// current document/evidence presentation is owned by the context sections.
	blocks := discoverAITagBlocks(content, "FACTS", "PLAN_FACTS", "EVIDENCE", "PLAN_EVIDENCE", "DOCUMENT", "PLAN_DOCUMENT")
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
			Start: start,
			End:   end,
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
			ID:      taskEvidenceID("verify", task),
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
			ID:      taskEvidenceID("summary", task),
			Content: fmt.Sprintf("[%s] 总结: %s", taskLabel, summary),
		},
	}
}

// Task IDs survive recovery and index changes. Plan-local indices can repeat
// in the next plan and must not overwrite another task's session evidence.
func taskEvidenceID(kind string, task *AiTask) string {
	id := strings.TrimSpace(task.TaskId)
	if id == "" {
		id = task.GetIndex()
	}
	return kind + "-" + id
}
