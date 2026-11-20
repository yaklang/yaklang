package aicommon

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

// ToolCallRuleGate inspects an explicit tool call before review and execution.
// Returning nil means no rule matched. The gate must be side-effect free; the
// ToolCaller owns approval, events and the final error.
type ToolCallRuleGate func(ctx context.Context, toolName string, params aitool.InvokeParams) *ToolCallRuleDecision

// ToolCallRuleMatch is one rule finding attached to a decision.
type ToolCallRuleMatch struct {
	RuleName string `json:"rule_name"`
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Risk     string `json:"risk"`
}

// ToolCallRuleDecision is the gate result for one tool call. The action comes
// from the shared severity mapping (high/critical=deny, middle=ask, low=warn).
type ToolCallRuleDecision struct {
	Action   schema.SyntaxFlowAction   `json:"action"`
	Severity schema.SyntaxFlowSeverity `json:"severity"`
	RuleName string                    `json:"rule_name"`
	RuleID   string                    `json:"rule_id"`
	Reason   string                    `json:"reason"`
	Matches  []ToolCallRuleMatch       `json:"matches,omitempty"`
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
// tool review step. It runs even for tools marked NoNeedUserReview so a deny
// decision cannot be skipped by internal-tool exemptions.
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
			// A broken analyzer must not take down the agent; fail open.
			log.Warnf("tool rule gate panicked for tool[%v]: %v", tool.Name, recovered)
			decision = nil
		}
	}()
	return t.ruleGate(t.ctx, tool.Name, params)
}
