package sfbuildin

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuiltinRuleMarkdownDescriptionAndSolution(t *testing.T) {
	var violations []string

	for _, fixture := range builtinFixtures(t) {
		path, rule := fixture.path, fixture.rule
		violations = append(violations, checkRuleMarkdown(path, "rule.description", rule.Description)...)
		violations = append(violations, checkRuleMarkdown(path, "rule.solution", rule.Solution)...)

		for alertName, alert := range rule.AlertDesc {
			if alert == nil {
				continue
			}
			violations = append(violations, checkRuleMarkdown(path, fmt.Sprintf("alert[%s].description", alertName), alert.Description)...)
			violations = append(violations, checkRuleMarkdown(path, fmt.Sprintf("alert[%s].solution", alertName), alert.Solution)...)
		}
	}

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
