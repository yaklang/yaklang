package base

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Loading uses RuleFS, but the historical exported path metadata is absolute
// relative to the current working directory. Keep that compatibility contract
// even when optimizing rule loading; it must not be cached with the document.
func TestParseRuleSourceDirectoryMetadata(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	for rule, want := range map[string]string{
		"ethernet.yaml":              cwd,
		"application-layer/ntp.yaml": filepath.Join(cwd, "application-layer"),
	} {
		t.Run(rule, func(t *testing.T) {
			ruleDocumentCache.Delete(rule)
			_, cached := ruleDocumentCache.Load(rule)
			require.False(t, cached)
			for i := 0; i < 2; i++ { // cold and cached documents have identical paths
				root, err := ParseRule(rule)
				require.NoError(t, err)
				_, cached = ruleDocumentCache.Load(rule)
				require.True(t, cached)
				require.Equal(t, want, root.Ctx.GetString("path"))
				require.True(t, filepath.IsAbs(root.Ctx.GetString("path")))
				root.Ctx.SetItem("path", "caller-owned mutation")
			}
		})
	}
	_, err = ParseRule("not-a-real-rule.yaml")
	require.Error(t, err)
}

func TestParseRuleWorkingDirectoryMetadata(t *testing.T) {
	const helper = "binparser-rule-path-test-child"
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == helper {
		cwd, err := os.Getwd()
		require.NoError(t, err)
		root, err := ParseRule("application-layer/ntp.yaml")
		require.NoError(t, err)
		require.Equal(t, filepath.Join(cwd, "application-layer"), root.Ctx.GetString("path"))
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		command := exec.Command(executable, "-test.run=^TestParseRuleWorkingDirectoryMetadata$", "-test.count=1", "--", helper)
		command.Dir = t.TempDir()
		output, err := command.CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
}
