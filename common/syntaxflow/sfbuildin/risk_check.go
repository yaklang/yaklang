//go:build !irify_exclude

package sfbuildin

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfrisk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// riskTypeErrors validates a parsed rule against an explicit taxonomy checker.
// Legacy aliases remain readable but cannot enter built-ins.
func riskTypeErrors(checker *sfrisk.Checker, rule *schema.SyntaxFlowRule) []string {
	if checker == nil {
		checker = sfrisk.DefaultChecker()
	}
	var errors []string
	check := func(field, value string) {
		if value != "" && !checker.IsCanonical(value) {
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

// builtinRiskTypeErrors keeps the embed-taxonomy behaviour exercised by the
// package unit tests.
func builtinRiskTypeErrors(rule *schema.SyntaxFlowRule) []string {
	return riskTypeErrors(sfrisk.DefaultChecker(), rule)
}

// CheckBuiltinRiskTypes walks a local rule directory and validates every .sf
// file against the given taxonomy. It never touches a database: the rules are
// read straight from the checkout, so a downloaded yak binary can run the same
// check that the repository's go test suite performs on its embedded copy.
func CheckBuiltinRiskTypes(dir string, checker *sfrisk.Checker) (*BuiltinRiskCheckResult, error) {
	result := &BuiltinRiskCheckResult{}
	types := make(map[string]bool)
	err := filesys.Recursive(dir, filesys.WithFileSystem(filesys.NewLocalFs()), filesys.WithFileStat(func(path string, info fs.FileInfo) error {
		if !strings.HasSuffix(info.Name(), ".sf") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return utils.Wrapf(err, "read rule file %s failed", path)
		}
		rule, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		if err != nil {
			result.Violations = append(result.Violations, fmt.Sprintf("%s: compile: %v", path, err))
			return nil
		}
		result.RuleCount++
		result.AlertCount += len(rule.AlertDesc)
		if rule.AllowIncluded {
			result.LibraryCount++
		}
		for _, problem := range riskTypeErrors(checker, rule) {
			result.Violations = append(result.Violations, path+": "+problem)
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
		return result, err
	}
	for typ := range types {
		result.CanonicalTypes = append(result.CanonicalTypes, typ)
	}
	sort.Strings(result.CanonicalTypes)
	if result.RuleCount == 0 || result.AlertCount == 0 {
		result.Violations = append(result.Violations, "no builtin rules or alerts were inspected")
	}
	return result, nil
}
