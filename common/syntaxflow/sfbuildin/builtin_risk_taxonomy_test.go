//go:build !irify_exclude

package sfbuildin

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfrisk"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// Validate canonical rule and alert risk types without imposing an alert
// identity scheme. Legacy aliases remain readable but cannot enter built-ins.
func builtinRiskTypeErrors(rule *schema.SyntaxFlowRule) []string {
	var errors []string
	check := func(field, value string) {
		if value != "" && !sfrisk.IsCanonical(value) {
			errors = append(errors, fmt.Sprintf("%s: noncanonical or review-required risk type %q", field, value))
		}
	}
	check("rule", rule.RiskType)
	for variable, alert := range rule.AlertDesc {
		if alert == nil {
			continue
		}
		check("alert["+variable+"]", alert.RiskType)
		if !rule.AllowIncluded && alert.RiskType == "" && rule.RiskType == "" {
			errors = append(errors, fmt.Sprintf("alert[%s]: missing effective risk type", variable))
		}
	}
	return errors
}

// Run default and gzip_embed builds against their actual embedded resource set.
func TestBuiltinRiskTypeTaxonomy(t *testing.T) {
	InitEmbedFSWithNotify(nil)
	var violations []string
	types := make(map[string]bool)
	rules, alerts, libraries := 0, 0, 0
	err := filesys.Recursive(".", filesys.WithFileSystem(ruleFSWithHash), filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if !strings.HasSuffix(info.Name(), ".sf") {
			return nil
		}
		raw, err := ruleFSWithHash.ReadFile(path)
		if err != nil {
			return err
		}
		rule, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: compile: %v", path, err))
			return nil
		}
		rules++
		alerts += len(rule.AlertDesc)
		if rule.AllowIncluded {
			libraries++
		}
		for _, problem := range builtinRiskTypeErrors(rule) {
			violations = append(violations, path+": "+problem)
		}
		if rule.RiskType != "" {
			types[rule.RiskType] = true
		}
		for _, alert := range rule.AlertDesc {
			if alert != nil && alert.RiskType != "" {
				types[alert.RiskType] = true
			}
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if rules == 0 || alerts == 0 {
		t.Fatal("no built-in rules or alerts were inspected")
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Fatalf("risk type governance violations:\n%s", strings.Join(violations, "\n"))
	}
	t.Logf("compiled %d rules (%d libraries), %d alerts, %d canonical types", rules, libraries, alerts, len(types))
}

func TestBuiltinRiskTypeValidation(t *testing.T) {
	const valid = `desc(risk: "sql-injection")
* as $sink
alert $sink`
	for _, tc := range []struct {
		name, source, problem string
	}{
		{"canonical default", valid, ""},
		{"legacy rule alias", strings.Replace(valid, "sql-injection", "SQL注入", 1), "noncanonical"},
		{"legacy alert alias", valid + `
* as $other
alert $other for {risk: "sqli"}`, "noncanonical"},
		{"misplaced severity", strings.Replace(valid, "sql-injection", "Moderate", 1), "noncanonical"},
		{"review placeholder", strings.Replace(valid, "sql-injection", "security-check", 1), "review-required"},
		{"unknown type", strings.Replace(valid, "sql-injection", "customer-risk", 1), "noncanonical"},
		{"missing effective type", strings.Replace(valid, `risk: "sql-injection"`, "", 1), "missing effective"},
		{"mixed alerts", `desc()
* as $first
* as $second
alert $first for {risk: "sql-injection"}
alert $second for {risk: "information"}`, ""},
		{"library outputs are not findings", `desc(lib: "helper-output")
* as $output
alert $output`, ""},
		{"last assignment invalid", strings.Replace(valid, `risk: "sql-injection"`, `risk: "sql-injection", risk_type: "sqli"`, 1), "noncanonical"},
		{"last assignment canonical", strings.Replace(valid, `risk: "sql-injection"`, `risk: "sqli", risk_type: "sql-injection"`, 1), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule, err := sfdb.CheckSyntaxFlowRuleContent(tc.source)
			if err != nil {
				t.Fatalf("fixture does not compile: %v", err)
			}
			problems := strings.Join(builtinRiskTypeErrors(rule), "\n")
			if tc.problem == "" && problems != "" || tc.problem != "" && !strings.Contains(problems, tc.problem) {
				t.Fatalf("want problem %q, got %q", tc.problem, problems)
			}
		})
	}
}
