package buildin_rule

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestBashRules(t *testing.T) {
	files := []string{
		filepath.Join("..", "bash", "cwe-78-os-command-injection", "bash-pipe-to-shell.sf"),
		filepath.Join("..", "bash", "cwe-78-os-command-injection", "bash-eval.sf"),
		filepath.Join("..", "bash", "cwe-78-os-command-injection", "bash-interpreter-c.sf"),
		filepath.Join("..", "bash", "cwe-78-os-command-injection", "bash-rm-rf-root.sf"),
		filepath.Join("..", "bash", "cwe-78-os-command-injection", "bash-dynamic-command.sf"),
	}

	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			require.NoError(t, err)
			require.NoError(t, ssatest.EvaluateVerifyFilesystem(string(raw), t, true))
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
