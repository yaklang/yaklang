package aicommon

import (
	"context"

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
	Action schema.SyntaxFlowAction `json:"action"`
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
