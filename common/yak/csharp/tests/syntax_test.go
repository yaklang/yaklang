package tests

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/csharp/csharp2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const savedCSharpFixtureMaxParseDuration = 30 * time.Second

//go:embed all:code
var codeFs embed.FS

func validateSource(t *testing.T, filename string, src string, caches ...*ssa.AntlrCache) {
	t.Helper()
	name := strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/")
	if name == "" {
		name = "inline.cs"
	}
	t.Run(fmt.Sprintf("syntax file: %v", name), func(t *testing.T) {
		var cache *ssa.AntlrCache
		if len(caches) > 0 {
			cache = caches[0]
		}
		start := time.Now()
		ast, err := csharp2ssa.Frontend(src, cache)
		require.NoError(t, err, "parse AST FrontEnd error")
		require.NotNil(t, ast)
		require.LessOrEqual(t, time.Since(start), savedCSharpFixtureMaxParseDuration, "parse took too long for %s", name)
	})
}

func TestAllSyntaxForCSharp_G4(t *testing.T) {
	builder, ok := csharp2ssa.CreateBuilder().(*csharp2ssa.SSABuilder)
	require.True(t, ok)
	defer builder.Clearup()
	cache := builder.GetAntlrCache()
	found := false
	err := fs.WalkDir(codeFs, "code", func(filePath string, d fs.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if d.IsDir() || !strings.HasSuffix(filePath, ".cs") {
			return nil
		}
		raw, err := codeFs.ReadFile(filePath)
		require.NoError(t, err)
		validateSource(t, filePath, string(raw), cache)
		found = true
		return nil
	})
	require.NoError(t, err)
	require.True(t, found, "no embed syntax files found")
}

func TestCSharpIfPreprocessor(t *testing.T) {
	src := `#define DEBUG
class C {
#if DEBUG
    int x = 1;
#else
    int x = 2;
#endif
}
`
	validateSource(t, "preprocessor.cs", src)
}

func TestCSharpNamespaceUsingClass(t *testing.T) {
	src := `using System;
namespace Demo.App {
    public class Box {
        public int W;
        public int H;
        public int Area() { return W * H; }
    }
}
`
	validateSource(t, "namespace.cs", src)
}
