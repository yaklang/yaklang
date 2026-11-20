package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func securityRuleReviewSummary(decision *ToolCallRuleDecision) string {
	if decision == nil {
		return ""
	}
	summary := strings.TrimSpace(decision.Summary())
	if summary == "" {
		summary = "security rule matched"
	}
	if severity := strings.TrimSpace(string(decision.Severity)); severity != "" {
		return fmt.Sprintf("[%s] %s", severity, summary)
	}
	return summary
}

// securityRuleReviewPrompt builds the visible human-review card. The frontend
// renders require_user_interactive prompts regardless of the yolo/auto policy,
// unlike tool_use_review_require cards.
func securityRuleReviewPrompt(decision *ToolCallRuleDecision, toolName string, params aitool.InvokeParams) string {
	var b strings.Builder
	b.WriteString("安全规则命中，需要人工确认\n\n")
	b.WriteString("工具：")
	b.WriteString(toolName)
	b.WriteString("\n")
	if decision != nil {
		if severity := strings.TrimSpace(string(decision.Severity)); severity != "" {
			b.WriteString("级别：")
			b.WriteString(severity)
			b.WriteString("\n")
		}
		if decision.RuleName != "" {
			b.WriteString("规则：")
			b.WriteString(decision.RuleName)
			b.WriteString("\n")
		}
		if decision.RuleID != "" {
			b.WriteString("规则ID：")
			b.WriteString(decision.RuleID)
			b.WriteString("\n")
		}
		if summary := strings.TrimSpace(decision.Summary()); summary != "" {
			b.WriteString("原因：")
			b.WriteString(summary)
			b.WriteString("\n")
		}
		for i, match := range decision.Matches {
			if i >= 3 {
				break
			}
			message := strings.TrimSpace(match.Message)
			if message == "" {
				message = strings.TrimSpace(match.RuleName)
			}
			if message != "" {
				b.WriteString("命中：")
				b.WriteString(message)
				b.WriteString("\n")
			}
		}
	}
	if toolName == "bash" {
		if command := strings.TrimSpace(utils.InterfaceToString(params["command"])); command != "" {
			b.WriteString("\n命令：\n")
			b.WriteString(command)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n是否继续执行？选择“取消”将终止本次工具调用。")
	return b.String()
}

func securityRuleReviewOptions() []map[string]any {
	return []map[string]any{
		{"index": 1, "prompt": "继续执行", "prompt_title": "继续执行"},
		{"index": 2, "prompt": "取消", "prompt_title": "取消"},
	}
}

// securityRuleReviewDecision only treats an explicit approval as consent.
// Unknown free text is rejected, because silence must never authorize a
// security-rule escalation.
func securityRuleReviewDecision(params aitool.InvokeParams) (bool, string) {
	text := ""
	if params != nil {
		text = strings.TrimSpace(params.GetString("suggestion"))
		if text == "" {
			text = strings.TrimSpace(utils.InterfaceToString(params))
		}
	}
	lower := strings.ToLower(text)
	for _, word := range []string{"取消", "cancel", "deny", "stop", "拒绝", "否", "不要", "别", "不执行"} {
		if strings.Contains(lower, word) {
			return false, "bash command cancelled by security rule review"
		}
	}
	for _, word := range []string{"继续", "continue", "允许", "approve", "yes", "同意", "确认", "执行"} {
		if strings.Contains(lower, word) {
			return true, ""
		}
	}
	return false, "bash command cancelled: security rule review did not explicitly approve execution"
}
