package commandcheck

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

//go:embed rules/*.sf
var guardRuleFS embed.FS

const (
	// maxCommandBytes bounds the SSA build. The AI's bash tool usually receives
	// short commands or small scripts; anything larger is allowed through
	// unchecked rather than stalling the tool call.
	maxCommandBytes = 64 * 1024

	disableEnv = "YAK_DISABLE_AI_BASH_COMMAND_GUARD"
)

var (
	guardOnce  sync.Once
	guardRules []*schema.SyntaxFlowRule
	guardErr   error
)

func loadGuardRules() {
	entries, err := fs.ReadDir(guardRuleFS, "rules")
	if err != nil {
		guardErr = fmt.Errorf("read embedded bash guard rules: %w", err)
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sf") {
			continue
		}
		raw, err := guardRuleFS.ReadFile("rules/" + entry.Name())
		if err != nil {
			guardErr = fmt.Errorf("read bash guard rule %s: %w", entry.Name(), err)
			return
		}
		rule, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		if err != nil {
			guardErr = fmt.Errorf("compile bash guard rule %s: %w", entry.Name(), err)
			return
		}
		guardRules = append(guardRules, rule)
	}
	if len(guardRules) == 0 {
		guardErr = fmt.Errorf("no embedded bash guard rules")
	}
}

// CheckCommand parses one shell command and runs the embedded high-severity
// bash rules against it. Parsing failures and checker failures are reported in
// Reason but never block execution: this is a guard against accidents, not a
// security boundary.
func CheckCommand(ctx context.Context, cmd string) Result {
	guardOnce.Do(loadGuardRules)

	if isGuardDisabled() {
		return Result{Available: guardErr == nil, Reason: "disabled"}
	}
	if guardErr != nil {
		log.Warnf("bash command guard unavailable: %v", guardErr)
		return Result{Available: false, Reason: "unavailable: " + guardErr.Error()}
	}

	cmd = normalizeCommand(cmd)
	if strings.TrimSpace(cmd) == "" {
		return Result{Available: true, Reason: "empty"}
	}
	if len(cmd) > maxCommandBytes {
		log.Warnf("bash command guard skipped oversized command: %d bytes", len(cmd))
		return Result{Available: true, Reason: "too_large"}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	prog, err := ssaapi.Parse(cmd,
		ssaapi.WithLanguage(ssaconfig.BASH),
		ssaapi.WithMemory(),
		ssaapi.WithContext(ctx),
	)
	if err != nil {
		log.Warnf("bash command guard skipped unparsable command: %v", err)
		return Result{Available: true, Reason: "parse_failed: " + err.Error()}
	}
	if prog.GetLanguage() != ssaconfig.BASH {
		log.Warnf("bash command guard skipped language mismatch: %s", prog.GetLanguage())
		return Result{Available: true, Reason: "language_mismatch: " + string(prog.GetLanguage())}
	}

	result := Result{Available: true}
	seen := make(map[string]struct{})
	for _, rule := range guardRules {
		res, err := prog.SyntaxFlowRule(rule)
		if err != nil {
			log.Warnf("bash command guard rule %s failed: %v", rule.RuleName, err)
			continue
		}
		res.GetAlertValues().ForEach(func(alertName string, values ssaapi.Values) bool {
			info, _ := rule.GetAlertInfo(alertName)
			for _, value := range values {
				finding := buildFinding(rule, info, alertName, value)
				key := fmt.Sprintf("%s|%s|%s|%d|%d", finding.RuleID, alertName, finding.Value, finding.Start, finding.End)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				result.Findings = append(result.Findings, finding)
			}
			return true
		})
	}
	result.Blocked = len(result.Findings) > 0
	return result
}

func buildFinding(rule *schema.SyntaxFlowRule, info *schema.SyntaxFlowDescInfo, alertName string, value *ssaapi.Value) Finding {
	finding := Finding{
		RuleID:  rule.RuleId,
		Level:   strings.ToLower(string(rule.Severity)),
		Title:   rule.Title,
		TitleZh: rule.TitleZh,
		Message: rule.Description,
		Value:   truncateValue(value.String(), 512),
	}
	if info != nil {
		if info.RuleId != "" {
			finding.RuleID = info.RuleId
		}
		if info.Title != "" {
			finding.Title = info.Title
		}
		if info.TitleZh != "" {
			finding.TitleZh = info.TitleZh
		}
		if info.Severity != "" {
			finding.Level = strings.ToLower(string(info.Severity))
		}
		switch {
		case info.Msg != "":
			finding.Message = info.Msg
		case info.Description != "":
			finding.Message = info.Description
		}
	}
	if finding.RuleID == "" {
		finding.RuleID = alertName
	}
	if r := value.GetRange(); r != nil {
		finding.Start = r.GetStartOffset()
		finding.End = r.GetEndOffset()
	}
	return finding
}

func normalizeCommand(cmd string) string {
	cmd = strings.ReplaceAll(cmd, "\r\n", "\n")
	return strings.ReplaceAll(cmd, "\r", "\n")
}

func isGuardDisabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(disableEnv))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func truncateValue(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.ToValidUTF8(value[:limit], "") + "..."
}
