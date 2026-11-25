package aireact

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func securityRuleGate(ctx context.Context, toolName string, params aitool.InvokeParams) *aicommon.ToolCallRuleDecision {
	return securityRuleGateWithOptions(nil)(ctx, toolName, params)
}

func securityRuleGateWithOptions(options []aicommon.BashRuleCheckOption) aicommon.ToolCallRuleGate {
	return func(ctx context.Context, toolName string, params aitool.InvokeParams) *aicommon.ToolCallRuleDecision {
		if !strings.EqualFold(strings.TrimSpace(toolName), "bash") {
			return nil
		}
		command := utils.InterfaceToString(params["command"])
		if strings.TrimSpace(command) == "" {
			return nil
		}

		check := aicommon.CheckBuiltinBashCommand(ctx, command, options...)
		if !check.Available {
			log.Warnf("bash rule permission check unavailable, allowing tool call")
			return nil
		}
		switch check.Action {
		case schema.SFR_ACTION_DENY, schema.SFR_ACTION_ASK, schema.SFR_ACTION_WARN:
			return &aicommon.ToolCallRuleDecision{Action: check.Action}
		default:
			return nil
		}
	}
}

// toolCallRuleGate hooks only the bash/shell tool and runs SyntaxFlow rules.
func toolCallRuleGate(cfg *aicommon.Config) aicommon.ToolCallRuleGate {
	if cfg == nil || cfg.DisableBuiltinSyntaxFlowRules {
		return nil
	}
	return securityRuleGateWithOptions([]aicommon.BashRuleCheckOption{
		aicommon.WithDisableBuiltinBashRules(false),
		aicommon.WithIncludeCustomBashRules(true),
	})
}
