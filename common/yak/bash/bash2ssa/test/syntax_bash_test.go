package test

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/yak/bash/bash2ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const (
	maxBashFixtureParseDuration = 10 * time.Second
	maxBashFixtureBuildDuration = 30 * time.Second
)

//go:embed all:code
var codeFs embed.FS

func isBashSyntaxFixture(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".sh", ".bash":
		return true
	default:
		return false
	}
}

func validateBashSource(t *testing.T, filename string, src []byte) {
	t.Helper()

	name := strings.TrimPrefix(path.Clean(strings.ReplaceAll(filename, "\\", "/")), "code/")
	t.Run(name, func(t *testing.T) {
		start := time.Now()
		_, parseErr := bash2ssa.CreateBuilder().ParseAST(string(src), nil)
		parseDur := time.Since(start)
		require.LessOrEqual(t, parseDur, maxBashFixtureParseDuration, "parse took too long for %s", name)

		if parseErr != nil {
			// Some upstream scripts carry zsh-only syntax that bash defers to
			// runtime but mvdan rejects while parsing. Keep them in the corpus,
			// and require the pipeline to fail cleanly instead of panicking.
			_, buildErr := ssaapi.Parse(string(src), ssaapi.WithLanguage(ssaconfig.BASH), ssaapi.WithMemory())
			require.Error(t, buildErr, "parse failure should surface from the build pipeline: %s", name)
			require.NotContains(t, strings.ToLower(buildErr.Error()), "panic", "fixture must fail cleanly: %s", name)
			t.Skipf("syntax unsupported by the bash parser: %v", parseErr)
		}

		start = time.Now()
		var prog *ssaapi.Program
		require.NotPanics(t, func() {
			var err error
			prog, err = ssaapi.Parse(string(src), ssaapi.WithLanguage(ssaconfig.BASH), ssaapi.WithMemory())
			require.NoError(t, err, "build fixture %s", name)
		}, "build fixture panicked: %s", name)
		buildDur := time.Since(start)
		require.LessOrEqual(t, buildDur, maxBashFixtureBuildDuration, "build took too long for %s", name)
		require.Equal(t, ssaconfig.BASH, prog.GetLanguage())

		res, err := prog.SyntaxFlowWithError(`*?{opcode: call} as $call`)
		require.NoError(t, err)
		calls := len(res.GetValues("call"))
		require.NotEmpty(t, res.GetValues("call"), "fixture %s built no calls", name)
		t.Logf("bash fixture=%s bytes=%d parse=%s build=%s calls=%d", name, len(src), parseDur.Round(time.Millisecond), buildDur.Round(time.Millisecond), calls)
	})
}

func TestAllSyntaxForBash_G4(t *testing.T) {
	err := fs.WalkDir(codeFs, "code", func(fixturePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !isBashSyntaxFixture(fixturePath) {
			return nil
		}
		raw, err := codeFs.ReadFile(fixturePath)
		if err != nil {
			return fmt.Errorf("read fixture %s: %w", fixturePath, err)
		}
		validateBashSource(t, fixturePath, raw)
		return nil
	})
	require.NoError(t, err, "walk bash syntax fixtures")
}
