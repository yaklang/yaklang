package aicommon

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// SecurityRuleDeniedError records an authoritative denial for the attempted
// operation. It is not a retryable parameter error: the ReAct layer must route
// it to security feedback so the model can tell a conflicting next step apart
// from unrelated work instead of aborting the whole task.
type SecurityRuleDeniedError struct {
	ToolName string
	RuleName string
	RuleID   string
	Severity schema.SyntaxFlowSeverity
	Reason   string
}

func (e *SecurityRuleDeniedError) Error() string {
	if e == nil {
		return "tool blocked by security rule"
	}
	parts := []string{fmt.Sprintf("tool[%s] blocked by security rule", e.ToolName)}
	if e.RuleName != "" {
		parts = append(parts, "rule="+e.RuleName)
	} else if e.RuleID != "" {
		parts = append(parts, "rule_id="+e.RuleID)
	}
	if e.Severity != "" {
		parts = append(parts, "severity="+string(e.Severity))
	}
	if e.Reason != "" {
		parts = append(parts, e.Reason)
	}
	return strings.Join(parts, " ")
}

// RetryFeedback is the model-facing feedback for the denied operation. Unlike
// directToolRetryFeedback it does not recommend correcting parameters; it asks
// the model to compare its next step with the denied operation.
func (e *SecurityRuleDeniedError) RetryFeedback() string {
	if e == nil {
		return SecurityRuleDeniedRetryFeedback("", "", "", "", "")
	}
	return SecurityRuleDeniedRetryFeedback(e.ToolName, e.RuleName, e.RuleID, e.Severity, e.Reason)
}

// IsSecurityRuleDenied reports whether err denies one operation by policy.
func IsSecurityRuleDenied(err error) bool {
	var target *SecurityRuleDeniedError
	return errors.As(err, &target)
}

// SecurityRuleDeniedRetryFeedback builds the feedback shared by gate denials
// and precheck results. The feedback carries the rule context and leaves the
// conflict decision to the model: only the denied operation is blocked, while
// unrelated work may continue. Keep the bypass list explicit because the model
// has previously tried other shells and wrappers after a denial.
func SecurityRuleDeniedRetryFeedback(toolName, ruleName, ruleID string, severity schema.SyntaxFlowSeverity, reason string) string {
	if strings.TrimSpace(toolName) == "" {
		toolName = "tool"
	}
	rule := strings.TrimSpace(ruleName)
	if rule == "" {
		rule = strings.TrimSpace(ruleID)
	}
	if rule == "" {
		rule = "unknown"
	}
	if strings.TrimSpace(reason) == "" {
		reason = "security policy denied the operation"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "安全策略拒绝了工具 %q 本次尝试的操作。这是对本次操作的拒绝，不是整个任务的终态。\n", toolName)
	fmt.Fprintf(&b, "命中规则：%s", rule)
	if ruleID != "" && ruleID != rule {
		fmt.Fprintf(&b, "（ID: %s）", ruleID)
	}
	if severity != "" {
		fmt.Fprintf(&b, "，级别：%s", severity)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "原因：%s\n", reason)
	b.WriteString("不要原样重试本次操作，也不要通过更换 shell（bash/cmd/powershell）、更换工具、拆分或改写命令、编码/转义/路径变形、写入脚本后再执行、改用 MCP/子智能体/后台任务，或重复调用等待策略变化来绕过。\n")
	b.WriteString("请自行判断下一步：如果下一步仍是同一操作、同一目标/效果，或目的就是绕过本次拒绝，停止该操作并向用户说明命中规则，等待新的授权或任务范围；如果下一步是无关的独立工作，只放弃本次被拒绝的命令，继续完成其余任务，不要因一次拒绝终止整个任务。无法判断是否冲突时，按冲突处理。")
	return b.String()
}
