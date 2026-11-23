package aireact

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

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
// It fails open when the analyzer itself is unavailable. An explicitly selected
// non-bash shell is never skipped: the command is first checked with the bash
// rules as a best-effort pass, then escalated to human review because the bash
// grammar cannot fully cover cmd or PowerShell.
func securityRuleGate(ctx context.Context, toolName string, params aitool.InvokeParams) *aicommon.ToolCallRuleDecision {
	if !strings.EqualFold(strings.TrimSpace(toolName), bashToolName) {
		return nil
	}
	command := utils.InterfaceToString(params["command"])
	if strings.TrimSpace(command) == "" {
		return nil
	}
	shell := normalizeShell(utils.InterfaceToString(params["shell"]))

	check := commandcheck.CheckBuiltinCommand(ctx, command)
	if !check.Available {
		log.Warnf("bash security rule evaluation unavailable, allowing tool call: %s", check.Reason)
		return nil
	}
	if check.Action == schema.SFR_ACTION_DENY || check.Action == schema.SFR_ACTION_ASK {
		return securityRuleDecisionFromCheck(check)
	}
	if shell == "bash" {
		if check.Action == schema.SFR_ACTION_WARN {
			return securityRuleDecisionFromCheck(check)
		}
		return nil
	}
	if check.Reason == "disabled" {
		return nil
	}
	return nonBashShellReviewDecision(shell, check)
}

// normalizeShell maps the bash tool's shell parameter to the grammar that will
// actually run. An empty value follows bash.yak's bash-first preference, so on
// Windows it is treated as non-bash when bash cannot be found at all.
func normalizeShell(shell string) string {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "bash", "bash.exe", "sh", "shell":
		return "bash"
	case "cmd", "cmd.exe":
		return "cmd"
	case "powershell", "powershell.exe", "ps", "pwsh", "pwsh.exe":
		return "powershell"
	case "":
		if runtime.GOOS == "windows" && !hasBashOnWindows() {
			return "auto-non-bash"
		}
		return "bash"
	default:
		return "unsupported"
	}
}

var (
	bashOnWindowsOnce sync.Once
	bashOnWindows     bool
)

// hasBashOnWindows mirrors the candidate list used by bash.yak so the gate
// agrees with the transport's auto-selection. The result is cached: the tool
// process does not install or remove a shell during a session.
func hasBashOnWindows() bool {
	bashOnWindowsOnce.Do(func() {
		var candidates []string
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			if base := strings.TrimSpace(os.Getenv(env)); base != "" {
				candidates = append(candidates,
					filepath.Join(base, "Git", "bin", "bash.exe"),
					filepath.Join(base, "Git", "usr", "bin", "bash.exe"),
				)
			}
		}
		if base := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); base != "" {
			candidates = append(candidates,
				filepath.Join(base, "Programs", "Git", "bin", "bash.exe"),
				filepath.Join(base, "Programs", "Git", "usr", "bin", "bash.exe"),
			)
		}
		if home := strings.TrimSpace(os.Getenv("USERPROFILE")); home != "" {
			candidates = append(candidates,
				filepath.Join(home, "scoop", "apps", "git", "current", "bin", "bash.exe"),
				filepath.Join(home, "scoop", "apps", "git", "current", "usr", "bin", "bash.exe"),
			)
		}
		candidates = append(candidates,
			`C:\msys64\usr\bin\bash.exe`,
			`C:\msys64\mingw64\bin\bash.exe`,
			`C:\cygwin64\bin\bash.exe`,
		)
		for _, candidate := range candidates {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				bashOnWindows = true
				return
			}
		}
	})
	return bashOnWindows
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

func nonBashShellReviewDecision(shell string, check commandcheck.Result) *aicommon.ToolCallRuleDecision {
	reason := fmt.Sprintf(
		"shell %q is not bash; built-in bash rules cannot fully analyze it, so human confirmation is required",
		shell,
	)
	if check.Action == schema.SFR_ACTION_WARN && strings.TrimSpace(check.Message()) != "" {
		reason += ": " + check.Message()
	}

	decision := securityRuleDecisionFromCheck(check)
	decision.Action = schema.SFR_ACTION_ASK
	decision.Severity = schema.SFR_SEVERITY_WARNING
	if strings.TrimSpace(decision.RuleID) == "" {
		decision.RuleID = "shell-static-analysis-unavailable"
	}
	if strings.TrimSpace(decision.RuleName) == "" {
		decision.RuleName = "Shell command requires human review"
	}
	decision.Reason = reason
	return decision
}

// toolCallRuleGate composes the built-in bash guard with the optional runtime
// gate compiled from Claude Code-style permissions JSON. Composition is
// strictest-wins, so a user allow rule cannot suppress a built-in deny or ask.
func toolCallRuleGate() aicommon.ToolCallRuleGate {
	extra := aicommon.CompileClaudeCodePermissionsFromRuntime()
	if extra == nil {
		return securityRuleGate
	}
	return aicommon.ComposeToolCallRuleGates(securityRuleGate, extra)
}
