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
// It fails open: analysis errors must not prevent the agent from working, and
// only a confident high/critical match blocks the call.
func securityRuleGate(ctx context.Context, toolName string, params aitool.InvokeParams) *aicommon.ToolCallRuleDecision {
	if !strings.EqualFold(strings.TrimSpace(toolName), bashToolName) {
		return nil
	}
	// The bash tool can delegate to cmd.exe or PowerShell. Their grammar is not
	// the one this gate evaluates, so leave those calls to their own guard.
	switch strings.ToLower(strings.TrimSpace(utils.InterfaceToString(params["shell"]))) {
	case "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return nil
	}
	command := utils.InterfaceToString(params["command"])
	if strings.TrimSpace(command) == "" {
		return nil
	}

	check := commandcheck.CheckBuiltinCommand(ctx, command)
	if !check.Available {
		log.Warnf("bash security rule evaluation unavailable, allowing tool call: %s", check.Reason)
		return nil
	}
	if check.Action == schema.SFR_ACTION_ALLOW {
		return nil
	}

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

// toolCallRuleGate composes the built-in bash guard with an optional gate
// compiled from Claude Code-style permissions JSON. Composition is strictest-
// wins, so a user allow rule cannot suppress a built-in deny or ask finding.
func (r *ReAct) toolCallRuleGate() aicommon.ToolCallRuleGate {
	if r == nil || r.config == nil {
		return securityRuleGate
	}
	extra := r.config.GetToolCallPermissionGate()
	if extra == nil {
		return securityRuleGate
	}
	return aicommon.ComposeToolCallRuleGates(securityRuleGate, extra)
}
