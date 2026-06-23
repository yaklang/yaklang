package sfbuildin

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

func TestBuiltinRuleMarkdownDescriptionAndSolution(t *testing.T) {
	var violations []string

	err := filesys.Recursive(".", filesys.WithFileSystem(ruleFSWithHash), filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		_, name := ruleFSWithHash.PathSplit(path)
		if !strings.HasSuffix(name, ".sf") {
			return nil
		}

		raw, err := ruleFSWithHash.ReadFile(path)
		if err != nil {
			return err
		}

		rule, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: failed to parse rule: %v", path, err))
			return nil
		}

		violations = append(violations, checkRuleMarkdown(path, "rule.description", rule.Description)...)
		violations = append(violations, checkRuleMarkdown(path, "rule.solution", rule.Solution)...)

		for alertName, alert := range rule.AlertDesc {
			if alert == nil {
				continue
			}
			violations = append(violations, checkRuleMarkdown(path, fmt.Sprintf("alert[%s].description", alertName), alert.Description)...)
			violations = append(violations, checkRuleMarkdown(path, fmt.Sprintf("alert[%s].solution", alertName), alert.Solution)...)
		}
		return nil
	}))
	require.NoError(t, err)

	if len(violations) > 0 {
		t.Fatalf("found %d builtin rule markdown issue(s):\n%s", len(violations), strings.Join(violations, "\n"))
	}
}

func checkRuleMarkdown(rulePath, fieldName, value string) []string {
	issues := validateMarkdownText(value)
	if len(issues) == 0 {
		return nil
	}
	violations := make([]string, 0, len(issues))
	for _, issue := range issues {
		violations = append(violations, fmt.Sprintf("%s (%s): %s", rulePath, fieldName, issue))
	}
	return violations
}
