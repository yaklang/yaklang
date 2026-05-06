package syntaxflow_utils

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// RunRuleContentOnProgram runs inlined rule text against the first SSA program matched by programName.
func RunRuleContentOnProgram(ctx context.Context, programName, ruleContent string) (*ssaapi.SyntaxFlowResult, error) {
	programName = strings.TrimSpace(programName)
	ruleContent = strings.TrimSpace(ruleContent)
	if programName == "" || ruleContent == "" {
		return nil, utils.Error("programName and ruleContent required")
	}
	progs := ssaapi.LoadProgramRegexp(programName)
	if len(progs) == 0 {
		return nil, fmt.Errorf("no SSA program loaded for name %q", programName)
	}
	rule, err := sfdb.CheckSyntaxFlowRuleContent(ruleContent)
	if err != nil {
		return nil, err
	}
	return progs[0].SyntaxFlowRule(rule, ssaapi.QueryWithContext(ctx))
}

// RunRuleFileOnProgram reads a .sf file and runs it on the program.
func RunRuleFileOnProgram(ctx context.Context, programName, rulePath string) (*ssaapi.SyntaxFlowResult, error) {
	raw, err := os.ReadFile(rulePath)
	if err != nil {
		return nil, err
	}
	return RunRuleContentOnProgram(ctx, programName, string(raw))
}

// FormatSyntaxFlowResultSummary returns a short text summary for AI feedback.
func FormatSyntaxFlowResultSummary(res *ssaapi.SyntaxFlowResult) string {
	if res == nil {
		return "(nil result)"
	}
	var sb strings.Builder
	sb.WriteString("SyntaxFlow result: ")
	if av := res.GetAlertValues(); av != nil {
		sb.WriteString(fmt.Sprintf("alerts=%d ", av.Len()))
	}
	sb.WriteString("(see engine output for full detail)\n")
	return sb.String()
}

// ParseRuleFile returns schema rule from disk path.
func ParseRuleFile(path string) (*schema.SyntaxFlowRule, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return sfdb.CheckSyntaxFlowRuleContent(string(raw))
}
