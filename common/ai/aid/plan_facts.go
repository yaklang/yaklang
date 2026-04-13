package aid

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const planFactsPersistentKey = "plan_facts"

func buildFactsBlock(facts string) string {
	facts = strings.TrimSpace(facts)
	if facts == "" {
		return ""
	}
	nonce := utils.RandStringBytes(6)
	return fmt.Sprintf("<|FACTS_%s|>\n%s\n<|FACTS_END_%s|>", nonce, facts, nonce)
}

func prependPlanFactsToRenderedPlan(base string, facts string) string {
	facts = strings.TrimSpace(facts)
	if facts == "" {
		return base
	}
	if strings.Contains(base, "<|FACTS_") || strings.Contains(base, "<|PLAN_FACTS_") {
		return base
	}
	block := buildFactsBlock(facts)
	if strings.TrimSpace(base) == "" {
		return block
	}
	return block + "\n\n" + base
}

func extractPlanFactsFromText(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	for _, prefix := range []string{"<|FACTS_", "<|PLAN_FACTS_"} {
		start := strings.Index(content, prefix)
		if start < 0 {
			continue
		}
		startLineEnd := strings.Index(content[start:], "|>")
		if startLineEnd < 0 {
			continue
		}
		bodyStart := start + startLineEnd + 2
		for bodyStart < len(content) && (content[bodyStart] == '\n' || content[bodyStart] == '\r') {
			bodyStart++
		}
		endPrefix := "<|FACTS_END_"
		if prefix == "<|PLAN_FACTS_" {
			endPrefix = "<|PLAN_FACTS_END_"
		}
		end := strings.Index(content[bodyStart:], endPrefix)
		if end < 0 {
			continue
		}
		return strings.TrimSpace(content[bodyStart : bodyStart+end])
	}
	return ""
}

func getTaskPlanFacts(task *AiTask) string {
	if task == nil {
		return ""
	}
	if task.Coordinator != nil && task.Coordinator.ContextProvider != nil {
		if facts, ok := task.Coordinator.ContextProvider.GetPersistentData(planFactsPersistentKey); ok {
			facts = strings.TrimSpace(facts)
			if facts != "" {
				return facts
			}
		}
	}
	root := task
	for root.ParentTask != nil {
		root = root.ParentTask
	}
	if root.AIStatefulTaskBase != nil {
		if facts := extractPlanFactsFromText(root.AIStatefulTaskBase.GetUserInput()); facts != "" {
			return facts
		}
	}
	return ""
}
