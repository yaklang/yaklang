//go:build !irify_exclude

package commandcheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
)

const builtinBashRuleDir = "../../../syntaxflow/sfbuildin/buildin/bash/cwe-78-os-command-injection"

// TestGuardRulesMatchBuiltinHighRules keeps the embedded copies aligned with the
// shipped high-severity bash rules. A new or demoted rule makes this test fail
// so the guard set is reviewed instead of drifting silently.
func TestGuardRulesMatchBuiltinHighRules(t *testing.T) {
	builtinHigh := make(map[string]string)
	err := filepath.WalkDir(builtinBashRuleDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sf") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rule, err := sfdb.CheckSyntaxFlowRuleContent(string(raw))
		if err != nil {
			return err
		}
		if strings.EqualFold(string(rule.Severity), "high") {
			builtinHigh[entry.Name()] = normalizeCommand(string(raw))
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, builtinHigh)

	embedded := make(map[string]string)
	entries, err := fs.ReadDir(guardRuleFS, "rules")
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sf") {
			continue
		}
		raw, err := guardRuleFS.ReadFile("rules/" + entry.Name())
		require.NoError(t, err)
		embedded[entry.Name()] = normalizeCommand(string(raw))
	}

	require.Equal(t, sortedRuleNames(builtinHigh), sortedRuleNames(embedded))
	for name, content := range builtinHigh {
		require.Equal(t, content, embedded[name], "embedded guard rule drifted from %s", name)
	}
}

func sortedRuleNames(rules map[string]string) []string {
	names := make([]string, 0, len(rules))
	for name := range rules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
