package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// SecurityAskContextKey tells DoWaitAgree that an AI-review failure must
// escalate to a human instead of auto-continuing.
const SecurityAskContextKey = "security_ask"

type securityReviewMode string

const (
	securityReviewNone  securityReviewMode = ""
	securityReviewHuman securityReviewMode = "human"
	securityReviewAI    securityReviewMode = "ai"
)

func agreePolicyFromConfig(cfg AICallerConfigIf) AgreePolicyType {
	if cfg == nil {
		return AgreePolicyManual
	}
	if provider, ok := cfg.(interface{ GetAgreePolicy() AgreePolicyType }); ok {
		return provider.GetAgreePolicy()
	}
	// Custom implementations that do not expose the policy keep the historical
	// manual behavior instead of silently auto-approving a security ask.
	return AgreePolicyManual
}

// resolveSecurityReviewAction applies the review policy to a rule action.
// Review policy controls interaction, not detection: deny stays a hard block,
// warn stays non-interactive, and ask is delegated to the policy's reviewer.
func resolveSecurityReviewAction(policy AgreePolicyType, action schema.SyntaxFlowAction) (schema.SyntaxFlowAction, securityReviewMode) {
	switch action {
	case schema.SFR_ACTION_DENY:
		return schema.SFR_ACTION_DENY, securityReviewNone
	case schema.SFR_ACTION_ASK:
		switch policy {
		case AgreePolicyYOLO, AgreePolicyAuto:
			// Non-interactive policies must not surface a security card. Keep
			// the finding visible as a warning and continue.
			return schema.SFR_ACTION_WARN, securityReviewNone
		case AgreePolicyAI, AgreePolicyAIAuto:
			return schema.SFR_ACTION_ASK, securityReviewAI
		default:
			return schema.SFR_ACTION_ASK, securityReviewHuman
		}
	case schema.SFR_ACTION_WARN:
		return schema.SFR_ACTION_WARN, securityReviewNone
	default:
		return schema.SFR_ACTION_ALLOW, securityReviewNone
	}
}

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

// securityRuleReviewPrompt builds the security review payload. It is shown as
// a human-interactive card in manual mode and as the review reason when an AI
// reviewer is asked to judge a middle-severity finding.
func securityRuleReviewPrompt(decision *ToolCallRuleDecision, toolName string, params aitool.InvokeParams) string {
	var b strings.Builder
	b.WriteString("安全规则命中，需要确认\n\n")
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
	if toolName == aitool.BashToolName {
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
		{"index": 1, "value": "continue", "prompt": "继续执行", "prompt_title": "继续执行"},
		{"index": 2, "value": "cancel", "prompt": "取消", "prompt_title": "取消"},
	}
}

// securityRuleReviewDecision only treats an explicit approval as consent.
// It reads decision fields rather than the whole params map, so command text
// such as "echo continue" can never be mistaken for a user approval.
func securityRuleReviewDecision(params aitool.InvokeParams) (bool, string) {
	const (
		cancelReason = "bash command cancelled by security rule review"
		noApproval   = "bash command cancelled: security rule review did not explicitly approve execution"
	)
	if params == nil {
		return false, noApproval
	}

	if index, ok := params["index"]; ok {
		switch utils.InterfaceToInt(index) {
		case 1:
			return true, ""
		case 2:
			return false, cancelReason
		}
	}

	for _, key := range []string{"suggestion", "value", "option_name", "prompt_title"} {
		text := strings.ToLower(strings.TrimSpace(strings.TrimRight(utils.InterfaceToString(params[key]), ":：")))
		switch text {
		case "continue", "allow", "approve", "yes", "1", "继续", "继续执行", "允许", "同意", "确认":
			return true, ""
		case "cancel", "deny", "reject", "no", "2", "取消", "拒绝", "不执行":
			return false, cancelReason
		}
	}
	return false, noApproval
}
