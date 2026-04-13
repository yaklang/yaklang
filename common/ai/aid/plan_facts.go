package aid

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	planFactsPersistentKey    = "plan_facts"
	planEvidencePersistentKey = "plan_evidence"
	planEvidenceTokenBudget   = 15000
)

var (
	planFactsBlockPattern        = regexp.MustCompile(`(?s)<\|FACTS_[^|]+\|>\s*(.*?)\s*<\|FACTS_END_[^|]+\|>`)
	legacyPlanFactsBlockPattern  = regexp.MustCompile(`(?s)<\|PLAN_FACTS_[^|]+\|>\s*(.*?)\s*<\|PLAN_FACTS_END_[^|]+\|>`)
	planEvidenceBlockPattern     = regexp.MustCompile(`(?s)<\|EVIDENCE_[^|]+\|>\s*(.*?)\s*<\|EVIDENCE_END_[^|]+\|>`)
	legacyPlanEvidenceBlockRegex = regexp.MustCompile(`(?s)<\|PLAN_EVIDENCE_[^|]+\|>\s*(.*?)\s*<\|PLAN_EVIDENCE_END_[^|]+\|>`)
)

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
	return extractPlanContextFromText(content, planFactsBlockPattern, legacyPlanFactsBlockPattern)
}

func extractPlanEvidenceFromText(content string) string {
	return extractPlanContextFromText(content, planEvidenceBlockPattern, legacyPlanEvidenceBlockRegex)
}

func extractPlanContextFromText(content string, patterns ...*regexp.Regexp) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	for _, pattern := range patterns {
		if pattern == nil {
			continue
		}
		matched := pattern.FindStringSubmatch(content)
		if len(matched) > 1 {
			return strings.TrimSpace(matched[1])
		}
	}
	return ""
}

func stripPlanContextBlocks(content string) string {
	content = strings.TrimSpace(content)
	for _, pattern := range []*regexp.Regexp{
		planFactsBlockPattern,
		legacyPlanFactsBlockPattern,
		planEvidenceBlockPattern,
		legacyPlanEvidenceBlockRegex,
	} {
		content = pattern.ReplaceAllString(content, "")
	}
	return strings.TrimSpace(content)
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
	incoming = strings.TrimSpace(incoming)
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
