package test

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/go2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

const savedGoFixtureMaxParseDuration = 30 * time.Second

//go:embed all:code
var codeFs embed.FS

var goTestAntlrCache = func() *ssa.AntlrCache {
	return go2ssa.CreateBuilder().GetAntlrCache()
}()

func isGoSyntaxASTFixture(fixturePath string) bool {
	return strings.EqualFold(filepath.Ext(fixturePath), ".go")
}

func goFixtureVirtualPath(filename string) string {
	trimmed := strings.TrimPrefix(filepath.ToSlash(filename), "code/")
	if trimmed == "" {
		trimmed = "fixture"
	}
	return path.Join("fixture", trimmed)
}

func validateBuildFromSource(t *testing.T, filename string, src string) {
	t.Helper()

	vf := filesys.NewVirtualFs()
	vf.AddFile("fixture/go.mod", "module fixture\n\ngo 1.20\n")
	vf.AddFile(goFixtureVirtualPath(filename), src)

	require.NotPanics(t, func() {
		progs, err := ssaapi.ParseProjectWithFS(
			vf,
			ssaapi.WithLanguage(ssaconfig.GO),
			ssaapi.WithMemory(),
		)
		require.NoError(t, err, "build from AST fixture failed: %s", filename)
		require.NotEmpty(t, progs)
	}, "build from AST fixture panicked: %s", filename)
}

func validateSource(t *testing.T, filename string, src string) {
	t.Helper()

	name := strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/")
	if name == "" {
		name = "inline.go"
	}

	t.Run(fmt.Sprintf("syntax file: %v", name), func(t *testing.T) {
		antlr4util.ResetSLLFirstCounters()

		start := time.Now()
		_, err := go2ssa.Frontend(src, goTestAntlrCache)
		parseDur := time.Since(start)
		require.NoError(t, err, "parse AST FrontEnd error")
		require.LessOrEqual(t, parseDur, savedGoFixtureMaxParseDuration, "parse took too long for %s", name)

		stats := antlr4util.SLLFirstCountersSnapshot()
		t.Logf("go fixture=%s parse=%s sll_attempts=%d fallbacks=%d cancelled=%d errors=%d", name, parseDur, stats.SLLAttempts, stats.Fallbacks, stats.FallbackCancelled, stats.FallbackError)

		validateBuildFromSource(t, filename, src)
	})
}

func TestAllSyntaxForGo_G4(t *testing.T) {
	err := fs.WalkDir(codeFs, "code", func(codePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isGoSyntaxASTFixture(codePath) {
			return nil
		}
		raw, err := codeFs.ReadFile(codePath)
		if err != nil {
			return fmt.Errorf("cannot found syntax fs %s: %w", codePath, err)
		}
		validateSource(t, codePath, string(raw))
		return nil
	})
	require.NoError(t, err, "walk go syntax fixtures")
}
