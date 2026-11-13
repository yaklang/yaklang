package sfanalysis

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	fi "github.com/yaklang/yaklang/common/utils/filesys"
)

// TestBuiltinHighSeverityRules_HaveFixtures is the corpus gate for severity
// evidence: every rule that can emit high/critical must carry at least one
// embedded positive fixture and one negative fixture. Verification itself
// (RunBuiltinRuleVerify / EvaluateVerifyFilesystemWithFrame) then proves the
// positive fires and the near-miss stays clean.
func TestBuiltinHighSeverityRules_HaveFixtures(t *testing.T) {
	root := BuiltinRuleRoot(t)
	local := fi.NewLocalFs()
	var missing []string
	var r, ok int
	err := fi.Recursive(root, fi.WithFileStat(func(path string, info fs.FileInfo) error {
		if info.IsDir() || !strings.HasSuffix(path, ".sf") {
			return nil
		}
		raw, err := local.ReadFile(path)
		if err != nil {
			return err
		}
		frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(string(raw))
		if err != nil {
			// Compilation is covered by the corpus compile test.
			return nil
		}
		rule := frame.GetRule()
		if !requiresHighSeverityEvidence(rule) {
			return nil
		}
		pos, _ := frame.ExtractVerifyFilesystemAndLanguage()
		neg, _ := frame.ExtractNegativeFilesystemAndLanguage()
		rel, _ := filepath.Rel(root, path)
		if len(pos) == 0 {
			missing = append(missing, rel+" (no positive fixture)")
		}
		if len(neg) == 0 {
			missing = append(missing, rel+" (no negative fixture)")
		}
		r++
		return nil
	}))
	require.NoError(t, err)
	ok = r - len(missing)
	_ = ok
	require.Greater(t, r, 0, "expected at least one high/critical builtin rule")
	require.Emptyf(t, missing, "high/critical rules must embed POS and NEG fixtures:\n%s",
		strings.Join(missing, "\n"))
}

// TestRequiresHighSeverityEvidence documents which rules fall under the gate.
func TestRequiresHighSeverityEvidence(t *testing.T) {
	require.False(t, requiresHighSeverityEvidence(nil))
	require.False(t, requiresHighSeverityEvidence(&schema.SyntaxFlowRule{
		Severity: schema.SFR_SEVERITY_WARNING,
	}))
	require.True(t, requiresHighSeverityEvidence(&schema.SyntaxFlowRule{
		Severity: schema.SFR_SEVERITY_HIGH,
	}))
	require.True(t, requiresHighSeverityEvidence(&schema.SyntaxFlowRule{
		Severity: schema.SFR_SEVERITY_INFO,
		AlertDesc: map[string]*schema.SyntaxFlowDescInfo{
			"hit": {Severity: schema.SFR_SEVERITY_CRITICAL},
		},
	}))
	require.False(t, requiresHighSeverityEvidence(&schema.SyntaxFlowRule{
		Severity:      schema.SFR_SEVERITY_CRITICAL,
		AllowIncluded: true,
	}))
	require.False(t, requiresHighSeverityEvidence(&schema.SyntaxFlowRule{
		Severity: schema.SFR_SEVERITY_INFO,
		AlertDesc: map[string]*schema.SyntaxFlowDescInfo{
			"hit": {Severity: schema.SFR_SEVERITY_HIGH, OnlyMsg: true},
		},
	}))
}
