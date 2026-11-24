package aireact

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/bash/commandcheck"
)

const (
	// bashToolName is the built-in shell tool backed by bash.yak. Other shell
	// tools (cmd, powershell) use different grammars and are not evaluable by
	// the bash frontend.
	bashToolName = "bash"

	maxSecurityRuleMatches = 8
)

// securityRuleGate turns built-in Bash rule findings into a tool-call decision.
// It fails open when the analyzer itself is unavailable. Non-bash shells are
// passed through for now; their own SyntaxFlow frontends will be added later.
func securityRuleGate(ctx context.Context, toolName string, params aitool.InvokeParams) *aicommon.ToolCallRuleDecision {
	return securityRuleGateWithOptions(nil)(ctx, toolName, params)
}

func securityRuleGateWithOptions(options []commandcheck.CheckOption) aicommon.ToolCallRuleGate {
	return func(ctx context.Context, toolName string, params aitool.InvokeParams) *aicommon.ToolCallRuleDecision {
		if !strings.EqualFold(strings.TrimSpace(toolName), bashToolName) {
			return nil
		}
		command := utils.InterfaceToString(params["command"])
		if strings.TrimSpace(command) == "" {
			return nil
		}
		// TODO: enable the bash rules for cmd/zsh once their frontends land.
		if !isBashShell(utils.InterfaceToString(params["shell"])) {
			return nil
		}

		check := commandcheck.CheckBuiltinCommand(ctx, command, options...)
		if !check.Available {
			log.Warnf("bash security rule evaluation unavailable, allowing tool call: %s", check.Reason)
			return nil
		}
		switch check.Action {
		case schema.SFR_ACTION_DENY, schema.SFR_ACTION_ASK, schema.SFR_ACTION_WARN:
			return securityRuleDecisionFromCheck(check)
		default:
			return nil
		}
	}
}

// isBashShell reports whether the bash tool's shell parameter selects the bash
// grammar. An empty value follows bash.yak's bash-first default.
func isBashShell(shell string) bool {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "", "bash", "bash.exe":
		return true
	default:
		return false
	}
}

func securityRuleDecisionFromCheck(check commandcheck.Result) *aicommon.ToolCallRuleDecision {
	decision := &aicommon.ToolCallRuleDecision{
		Action:   check.Action,
		Severity: check.Severity,
		Reason:   check.Message(),
	}
	if top := check.TopFinding(); top != nil {
		decision.RuleID = top.RuleID
		decision.RuleName = top.Title
		if decision.RuleName == "" {
			decision.RuleName = top.RuleID
		}
	}
	for i, finding := range check.Findings {
		if i >= maxSecurityRuleMatches {
			break
		}
		decision.Matches = append(decision.Matches, aicommon.ToolCallRuleMatch{
			RuleName: finding.Title,
			RuleID:   finding.RuleID,
			Severity: string(finding.Severity),
			Message:  finding.Message,
			Risk:     finding.Risk,
		})
	}
	return decision
}

// toolCallRuleGate is the single command-security entry point. When the
// session switch is off it returns nil so neither built-in nor custom rows run.
// When it is on, all built-in rules participate and stored custom rows are
// included only when their per-row switch is enabled. Optional runtime
// permission rules are composed under the same master switch.
func toolCallRuleGate(cfg *aicommon.Config) aicommon.ToolCallRuleGate {
	if cfg == nil || cfg.DisableBuiltinSyntaxFlowRules {
		return nil
	}
	options := []commandcheck.CheckOption{
		commandcheck.WithDisableBuiltinRules(false),
		commandcheck.WithIncludeCustomRules(true),
	}
	builtinGate := securityRuleGateWithOptions(options)
	extra := aicommon.CompileClaudeCodePermissionsFromRuntime()
	if extra == nil {
		return builtinGate
	}
	return aicommon.ComposeToolCallRuleGates(builtinGate, extra)
}
