package tests

import (
	"embed"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/csharp/asp"
)

const savedASPFrontFixtureMaxParseDuration = 30 * time.Second

//go:embed all:code
var aspFs embed.FS

func TestAllASPFrontFixtures(t *testing.T) {
	found := false
	err := fs.WalkDir(aspFs, "code", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() {
			return nil
		}
		lower := strings.ToLower(filePath)
		if !strings.HasSuffix(lower, ".aspx") && !strings.HasSuffix(lower, ".asp") {
			return nil
		}
		raw, err := aspFs.ReadFile(filePath)
		require.NoError(t, err)
		t.Run(filePath, func(t *testing.T) {
			start := time.Now()
			ast, err := asp.Front(string(raw))
			require.NoError(t, err, "parse ASP AST FrontEnd error")
			require.NotNil(t, ast)
			require.LessOrEqual(t, time.Since(start), savedASPFrontFixtureMaxParseDuration)
		})
		found = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, found, "no embed asp fixtures found")
}

func TestASPScriptletAndEcho(t *testing.T) {
	src := `<html><body><% int x = 1; %><%= x %></body></html>`
	ast, err := asp.Front(src)
	require.NoError(t, err)
	require.NotNil(t, ast)
}
