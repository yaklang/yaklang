//go:build !irify_exclude

package sfbuildin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/syntaxflow/sfrisk"
)

func makeRiskChecker(t *testing.T, taxonomyFile string) *sfrisk.Checker {
	t.Helper()
	data, err := os.ReadFile(taxonomyFile)
	require.NoError(t, err)
	checker, err := sfrisk.NewChecker(data)
	require.NoError(t, err)
	return checker
}

func TestCheckBuiltinRiskTypesFromLocalFS(t *testing.T) {
	checker := makeRiskChecker(t, "../sfrisk/taxonomy.json")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "valid.sf"), []byte(`desc(risk: "sql-injection")
* as $sink
alert $sink`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "invalid-alias.sf"), []byte(`desc(risk: "SQL注入")
* as $sink
alert $sink`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.sf"), []byte(`desc(`), 0o644))

	result, err := CheckBuiltinRiskTypes(dir, checker)
	require.NoError(t, err)
	require.Equal(t, 2, result.RuleCount)
	require.Equal(t, 2, result.AlertCount)
	require.Equal(t, []string{"SQL注入", "sql-injection"}, result.CanonicalTypes)
	require.Contains(t, strings.Join(result.Violations, "\n"), "invalid-alias.sf: rule: noncanonical")
	require.Contains(t, strings.Join(result.Violations, "\n"), "broken.sf: compile:")
}

func TestCheckBuiltinRiskTypesEmptyDir(t *testing.T) {
	checker := makeRiskChecker(t, "../sfrisk/taxonomy.json")
	result, err := CheckBuiltinRiskTypes(t.TempDir(), checker)
	require.NoError(t, err)
	require.Equal(t, 0, result.RuleCount)
	require.Contains(t, strings.Join(result.Violations, "\n"), "no builtin rules or alerts were inspected")
}
