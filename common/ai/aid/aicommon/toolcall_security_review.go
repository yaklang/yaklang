package aicommon

import (
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
	return AgreePolicyManual
}

// resolveSecurityReviewAction applies the review policy to a rule action.
func resolveSecurityReviewAction(policy AgreePolicyType, action schema.SyntaxFlowAction) (schema.SyntaxFlowAction, securityReviewMode) {
	switch action {
	case schema.SFR_ACTION_DENY:
		return schema.SFR_ACTION_DENY, securityReviewNone
	case schema.SFR_ACTION_ASK:
		switch policy {
		case AgreePolicyYOLO, AgreePolicyAuto:
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
		return string(decision.Action)
	}
	return summary
}

func securityRuleReviewPrompt(decision *ToolCallRuleDecision, toolName string, params aitool.InvokeParams) string {
	var b strings.Builder
	b.WriteString("规则权限策略命中，需要确认\n\n")
	b.WriteString("工具：")
	b.WriteString(toolName)
	b.WriteString("\n")
	if decision != nil {
		b.WriteString("判定：")
		b.WriteString(verdictLabel(decision.Action))
		b.WriteString("\n")
		if decision.RuleName != "" {
			b.WriteString("规则：")
			b.WriteString(decision.RuleName)
			b.WriteString("\n")
		}
		if summary := strings.TrimSpace(decision.Summary()); summary != "" {
			b.WriteString("原因：")
			b.WriteString(summary)
			b.WriteString("\n")
		}
	}
	if toolName == "bash" {
		if cmd := strings.TrimSpace(utils.InterfaceToString(params["command"])); cmd != "" {
			b.WriteString("\n命令：\n")
			b.WriteString(cmd)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n是否继续执行？选择“取消”将终止本次工具调用。")
	return b.String()
}

func verdictLabel(action schema.SyntaxFlowAction) string {
	switch action {
	case schema.SFR_ACTION_DENY:
		return "拒绝"
	case schema.SFR_ACTION_ASK:
		return "询问"
	case schema.SFR_ACTION_WARN:
		return "警告"
	default:
		return "通过"
	}
}

func securityRuleReviewOptions() []map[string]any {
	return []map[string]any{
		{"index": 1, "value": "continue", "prompt": "继续执行", "prompt_title": "继续执行"},
		{"index": 2, "value": "cancel", "prompt": "取消", "prompt_title": "取消"},
	}
}

// securityRuleReviewDecision only treats an explicit approval as consent.
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
