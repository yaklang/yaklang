package aicommon

import (
	"errors"
	"fmt"
	"strings"
)

// SecurityRuleDeniedError records an authoritative denial for the attempted
// operation.
type SecurityRuleDeniedError struct {
	ToolName string
	Reason   string
}

func (e *SecurityRuleDeniedError) Error() string {
	if e == nil {
		return "tool blocked by rule permission policy"
	}
	message := fmt.Sprintf("tool[%s] denied by rule permission policy", e.ToolName)
	if reason := strings.TrimSpace(e.Reason); reason != "" {
		message += " " + reason
	}
	return message
}

func (e *SecurityRuleDeniedError) RetryFeedback() string {
	if e == nil {
		return SecurityRuleDeniedRetryFeedback("", "")
	}
	return SecurityRuleDeniedRetryFeedback(e.ToolName, e.Reason)
}

func IsSecurityRuleDenied(err error) bool {
	var target *SecurityRuleDeniedError
	return errors.As(err, &target)
}

func SecurityRuleDeniedRetryFeedback(toolName, reason string) string {
	if strings.TrimSpace(toolName) == "" {
		toolName = "tool"
	}
	if strings.TrimSpace(reason) == "" {
		reason = "rule permission policy denied the operation"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "规则权限策略拒绝了工具 %q 本次尝试的操作。这是对本次操作的拒绝，不是整个任务的终态。\n", toolName)
	fmt.Fprintf(&b, "判定：拒绝\n")
	fmt.Fprintf(&b, "原因：%s\n", reason)
	b.WriteString("不要原样重试本次操作，也不要通过更换 shell（bash/cmd/powershell）、更换工具、拆分或改写命令、编码/转义/路径变形、写入脚本后再执行、改用 MCP/子智能体/后台任务，或重复调用等待策略变化来绕过。\n")
	b.WriteString("请自行判断下一步：如果下一步仍是同一操作、同一目标/效果，或目的就是绕过本次拒绝，停止该操作并向用户说明；如果下一步是无关的独立工作，只放弃本次被拒绝的命令，继续完成其余任务。无法判断是否冲突时，按冲突处理。")
	return b.String()
}
