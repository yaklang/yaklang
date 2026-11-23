//go:build !irify_exclude

package yakcmds_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/cmd/yakcmds"
)

func TestCodeScanAllLanguageConflictsWithExplicitLanguage(t *testing.T) {
	for _, flag := range []string{"-l", "--language"} {
		for _, language := range []string{"java", ""} {
			app := cli.NewApp()
			addCommands(app, yakcmds.SSACompilerCommands...)
			err := app.Run([]string{"yak", "code-scan", "--all-language", flag, language,
				"--debug", filepath.Join(t.TempDir(), "debug")})
			require.ErrorContains(t, err, "mutually exclusive")
			require.ErrorContains(t, err, "--all-language")
			require.ErrorContains(t, err, "--language/-l")
		}
	}
}
