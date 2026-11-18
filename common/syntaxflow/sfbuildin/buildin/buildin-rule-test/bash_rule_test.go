package buildin_rule

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

type bashRule struct {
	name    string
	content string
}

func loadBashRules(t *testing.T) []bashRule {
	t.Helper()
	root := filepath.Join("..", "bash")
	var rules []bashRule
	err := filepath.WalkDir(root, func(rulePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sf") {
			return nil
		}
		raw, err := os.ReadFile(rulePath)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, rulePath)
		if err != nil {
			name = rulePath
		}
		rules = append(rules, bashRule{name: filepath.ToSlash(name), content: string(raw)})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, rules)
	return rules
}

func TestBashRules(t *testing.T) {
	for _, rule := range loadBashRules(t) {
		rule := rule
		t.Run(rule.name, func(t *testing.T) {
			require.NoError(t, ssatest.EvaluateVerifyFilesystem(rule.content, t, true))
		})
	}

	// Loop items are unrolled for literal lists. Every item must reach the
	// rule, not just the first one, while non-literal lists stay unknown.
	t.Run("rm-rf-root-for-loop", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join("..", "bash", "cwe-78-os-command-injection", "bash-rm-rf-root.sf"))
		require.NoError(t, err)
		cases := []struct {
			code string
			want int
		}{
			{`for p in / /tmp/build; do rm -rf "$p"; done`, 1},
			{`for p in /tmp/build /; do rm -rf "$p"; done`, 1},
			{`for p in /tmp/build /var/tmp; do rm -rf "$p"; done`, 0},
			{`for p in "$@"; do rm -rf "$p"; done`, 0},
		}
		for _, tc := range cases {
			prog, err := ssaapi.Parse(tc.code, ssaapi.WithLanguage(ssaconfig.BASH), ssaapi.WithMemory())
			require.NoError(t, err)
			res, err := prog.SyntaxFlowWithError(string(raw))
			require.NoError(t, err)
			hits := 0
			for _, name := range res.GetAlertVariables() {
				hits += len(res.GetValues(name))
			}
			require.Equalf(t, tc.want, hits, "code=%s", tc.code)
		}
	})
}
