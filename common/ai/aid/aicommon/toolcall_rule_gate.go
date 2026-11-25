package aicommon

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// ToolCallRuleGate inspects an explicit tool call before review and execution.
// Returning nil means no rule matched (equivalent to allow).
type ToolCallRuleGate func(ctx context.Context, toolName string, params aitool.InvokeParams) *ToolCallRuleDecision

// ToolCallRuleDecision is the gate result for one tool call.
// Action is the only verdict: allow / warn / ask / deny.
type ToolCallRuleDecision struct {
	Action   schema.SyntaxFlowAction `json:"action"`
	RuleName string                  `json:"rule_name,omitempty"`
	RuleID   string                  `json:"rule_id,omitempty"`
	Reason   string                  `json:"reason,omitempty"`
}

// Summary is a compact human-readable description for logs and review cards.
func (d *ToolCallRuleDecision) Summary() string {
	if d == nil {
		return ""
	}
	message := strings.TrimSpace(d.Reason)
	if message == "" {
		message = strings.TrimSpace(d.RuleName)
	}
	if message == "" {
		message = string(d.Action)
	}
	return message
}

// WithToolCaller_RuleGate installs a deterministic security gate before the
// tool review step.
func WithToolCaller_RuleGate(gate ToolCallRuleGate) ToolCallerOption {
	return func(tc *ToolCaller) {
		tc.ruleGate = gate
	}
}

func (t *ToolCaller) applyToolCallRuleGate(tool *aitool.Tool, params aitool.InvokeParams) (decision *ToolCallRuleDecision) {
	if t == nil || t.ruleGate == nil || tool == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Warnf("tool rule gate panicked for tool[%v]: %v", tool.Name, recovered)
			decision = nil
		}
	}()
	return t.ruleGate(t.ctx, tool.Name, params)
}
