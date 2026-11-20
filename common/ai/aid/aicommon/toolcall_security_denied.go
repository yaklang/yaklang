package aicommon

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
)

// SecurityRuleDeniedError is a terminal policy decision, not a retryable tool
// failure. The ReAct layer must not feed it into the generic "fix and retry"
// path, because retrying with another shell/tool/encoding is exactly the
// bypass this type exists to stop.
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

// RetryFeedback is the model-facing terminal instruction. It intentionally
// says "stop", unlike directToolRetryFeedback which says "fix and retry".
func (e *SecurityRuleDeniedError) RetryFeedback() string {
	if e == nil {
		return SecurityRuleDeniedRetryFeedback("", "", "", "", "")
	}
	return SecurityRuleDeniedRetryFeedback(e.ToolName, e.RuleName, e.RuleID, e.Severity, e.Reason)
}

// IsSecurityRuleDenied reports whether err is a terminal security decision.
func IsSecurityRuleDenied(err error) bool {
	var target *SecurityRuleDeniedError
	return errors.As(err, &target)
}

// SecurityRuleDeniedRetryFeedback builds the terminal instruction shared by
// gate denials and precheck results. Keep the bypass list explicit: the model
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
	fmt.Fprintf(&b, "安全策略已拒绝工具 %q，这是终态，不是可重试的参数错误。\n", toolName)
	fmt.Fprintf(&b, "命中规则：%s", rule)
	if ruleID != "" && ruleID != rule {
		fmt.Fprintf(&b, "（ID: %s）", ruleID)
	}
	if severity != "" {
		fmt.Fprintf(&b, "，级别：%s", severity)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "原因：%s\n", reason)
	b.WriteString("禁止通过以下方式绕过：更换 shell（bash/cmd/powershell）、更换工具、拆分或改写命令、编码/转义/路径变形、写入脚本后再执行、改用 MCP/子智能体/后台任务，或重复调用等待策略变化。\n")
	b.WriteString("立即停止该操作，向用户简要说明命中的规则和命令未执行；只有用户明确给出新的授权或新的任务范围后才可继续。")
	return b.String()
}
