package aireact

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

// securityRuleGate hooks only the bash tool and returns ask/warn/deny verdicts.
func securityRuleGate(ctx context.Context, toolName string, params aitool.InvokeParams) *aicommon.ToolCallRuleDecision {
	if !strings.EqualFold(strings.TrimSpace(toolName), "bash") {
		return nil
	}
	command := utils.InterfaceToString(params["command"])
	if strings.TrimSpace(command) == "" {
		return nil
	}

	check := aicommon.CheckBashCommand(ctx, command)
	switch check.Action {
	case schema.SFR_ACTION_DENY, schema.SFR_ACTION_ASK, schema.SFR_ACTION_WARN:
		return &aicommon.ToolCallRuleDecision{Action: check.Action}
	default:
		return nil
	}
}

// toolCallRuleGate hooks only the bash/shell tool and honors the master switch.
func toolCallRuleGate(cfg *aicommon.Config) aicommon.ToolCallRuleGate {
	if cfg == nil || cfg.DisableBuiltinSyntaxFlowRules {
		return nil
	}
	return securityRuleGate
}
